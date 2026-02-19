package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLatitudeLongitude      = "40.3458;-3.8249"
	defaultTempThresholdDrift     = 2.0
	defaultPrecipThresholdPercent = 30.0
	defaultPrecipThresholdMM      = 2.0
	defaultTimeRange              = "08:00-20:00"
	dateLayout                    = "2006-01-02"
	timeLayout                    = "15:04"
	dateTimeLayout                = "2006-01-02T15:04"
	openMeteoGeoEndpoint          = "https://geocoding-api.open-meteo.com/v1/search"
	openMeteoForecastEndpoint     = "https://api.open-meteo.com/v1/forecast"
	logColorReset                 = "\033[0m"
	logColorBlue                  = "\033[1;34m"
	logColorYellow                = "\033[1;33m"
	logColorRed                   = "\033[1;31m"
	logColorCyan                  = "\033[1;36m"
)

type logLevel int

const (
	levelDebug logLevel = iota
	levelInfo
	levelWarning
	levelError
)

type Logger struct {
	level logLevel
	out   io.Writer
}

func newLogger(levelText string, out io.Writer) (*Logger, error) {
	parsed, err := parseLogLevel(levelText)
	if err != nil {
		return nil, err
	}
	return &Logger{level: parsed, out: out}, nil
}

func parseLogLevel(levelText string) (logLevel, error) {
	switch strings.ToUpper(strings.TrimSpace(levelText)) {
	case "DEBUG":
		return levelDebug, nil
	case "INFO", "":
		return levelInfo, nil
	case "WARNING", "WARN":
		return levelWarning, nil
	case "ERROR":
		return levelError, nil
	default:
		return levelInfo, fmt.Errorf("invalid log level %q", levelText)
	}
}

func (l *Logger) Debugf(format string, args ...any) {
	l.logf(levelDebug, "DEBUG", logColorCyan, format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.logf(levelInfo, "INFO", logColorBlue, format, args...)
}

func (l *Logger) Warningf(format string, args ...any) {
	l.logf(levelWarning, "WARNING", logColorYellow, format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.logf(levelError, "ERROR", logColorRed, format, args...)
}

func (l *Logger) logf(level logLevel, label, color, format string, args ...any) {
	if l == nil || level < l.level {
		return
	}
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.out, "%s[%s]%s %s\n", color, label, logColorReset, message)
}

type Config struct {
	LatitudeLongitude         string
	Date                      string
	TemperatureThresholdDrift float64
	PrecipitationThresholdPct float64
	PrecipitationThresholdMM  float64
	TimeRange                 string
	LogLevel                  string
	DefaultApplied            map[string]bool
}

type TimeRange struct {
	StartMinute int
	EndMinute   int
}

type Location struct {
	Name      string
	Latitude  float64
	Longitude float64
	Timezone  string
}

type HourlyPoint struct {
	Time              time.Time
	TemperatureC      float64
	PrecipitationProb float64
	PrecipitationMM   float64
}

type WeatherService interface {
	ResolveLocation(ctx context.Context, query string) (Location, error)
	HourlyForecast(ctx context.Context, loc Location, startDate time.Time, endDate time.Time) ([]HourlyPoint, error)
}

type OpenMeteoService struct {
	HTTPClient *http.Client
	Logger     *Logger
}

type Report struct {
	TargetDate       time.Time
	DayName          string
	MaxTempChange    float64
	MaxTempTarget    float64
	MinTempChange    float64
	MinTempTarget    float64
	HasMaxTempChange bool
	HasMinTempChange bool
	SignificantRains []HourlyPoint
}

func main() {
	ctx := context.Background()
	service := &OpenMeteoService{HTTPClient: &http.Client{Timeout: 15 * time.Second}}
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, service); err != nil {
		fmt.Fprintf(os.Stderr, "%s[ERROR]%s %v\n", logColorRed, logColorReset, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, service WeatherService) error {
	config, helpShown, parseErr := parseArgs(args, stdout)
	if helpShown {
		return nil
	}
	if parseErr != nil {
		return parseErr
	}

	logger, err := newLogger(config.LogLevel, stderr)
	if err != nil {
		return err
	}
	if openMeteo, ok := service.(*OpenMeteoService); ok {
		openMeteo.Logger = logger
	}

	date, err := parseDateOrTomorrow(config.Date, time.Now())
	if err != nil {
		return err
	}
	rangeWindow, err := parseTimeRange(config.TimeRange)
	if err != nil {
		return err
	}

	logStartupArgs(logger, config, date)
	logger.Infof("Starting weather check")

	if strings.TrimSpace(config.LatitudeLongitude) == "" {
		return errors.New("--latitude-longitude cannot be empty")
	}

	loc, err := service.ResolveLocation(ctx, config.LatitudeLongitude)
	if err != nil {
		logger.Errorf("Could not resolve location %q: %v", config.LatitudeLongitude, err)
		return err
	}
	logger.Infof("Resolved location %s (lat=%.4f lon=%.4f tz=%s)", loc.Name, loc.Latitude, loc.Longitude, loc.Timezone)

	serviceStart := date.AddDate(0, 0, -1)
	serviceEnd := date
	hourly, err := service.HourlyForecast(ctx, loc, serviceStart, serviceEnd)
	if err != nil {
		logger.Errorf("Could not fetch hourly forecast: %v", err)
		return err
	}
	logger.Infof("Fetched %d hourly records", len(hourly))

	report, err := analyze(hourly, date, rangeWindow, config.TemperatureThresholdDrift, config.PrecipitationThresholdPct, config.PrecipitationThresholdMM)
	if err != nil {
		logger.Errorf("Could not analyze weather data: %v", err)
		return err
	}

	logger.Infof("Computed report: max-change=%s min-change=%s rains=%d", boolToStatus(report.HasMaxTempChange), boolToStatus(report.HasMinTempChange), len(report.SignificantRains))
	if hasResults(report) {
		_, writeErr := fmt.Fprintln(stdout, renderReport(report))
		if writeErr != nil {
			return writeErr
		}
	} else {
		logger.Infof("No significant changes found; skipping stdout output")
	}

	logger.Infof("Completed weather check")
	return nil
}

func boolToStatus(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func logStartupArgs(logger *Logger, config Config, resolvedDate time.Time) {
	labels := map[string]string{
		"latitude-longitude":              config.LatitudeLongitude,
		"date":                            resolvedDate.Format(dateLayout),
		"temperature-threshold-drift":     fmt.Sprintf("%s", compactNumber(config.TemperatureThresholdDrift)),
		"precipitation-threshold_percent": fmt.Sprintf("%s", compactNumber(config.PrecipitationThresholdPct)),
		"precipitation-threshold_mm":      fmt.Sprintf("%s", compactNumber(config.PrecipitationThresholdMM)),
		"time-range":                      config.TimeRange,
		"log-level":                       config.LogLevel,
	}

	order := []string{
		"latitude-longitude",
		"date",
		"temperature-threshold-drift",
		"precipitation-threshold_percent",
		"precipitation-threshold_mm",
		"time-range",
		"log-level",
	}

	defaultApplied := make([]string, 0)
	for _, key := range order {
		source := "user"
		if config.DefaultApplied[key] {
			source = "default"
			defaultApplied = append(defaultApplied, "--"+key)
		}
		logger.Infof("Arg --%s=%q (%s)", key, labels[key], source)
	}

	if len(defaultApplied) == 0 {
		logger.Infof("Defaults applied: none")
		return
	}
	logger.Infof("Defaults applied: %s", strings.Join(defaultApplied, ", "))
}

func hasResults(report Report) bool {
	return report.HasMaxTempChange || report.HasMinTempChange || len(report.SignificantRains) > 0
}

func parseArgs(args []string, stdout io.Writer) (Config, bool, error) {
	config := Config{}
	config.DefaultApplied = map[string]bool{}
	flagSet := flag.NewFlagSet("weather_check", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	latitudeLongitude := flagSet.String("latitude-longitude", defaultLatitudeLongitude, "Location in latitude;longitude format")
	date := flagSet.String("date", "", "Target date (YYYY-MM-DD), defaults to next day")
	tempDrift := flagSet.Float64("temperature-threshold-drift", defaultTempThresholdDrift, "Significant temperature drift in Celsius")
	precipPct := flagSet.Float64("precipitation-threshold_percent", defaultPrecipThresholdPercent, "Significant precipitation probability threshold")
	precipMM := flagSet.Float64("precipitation-threshold_mm", defaultPrecipThresholdMM, "Significant precipitation amount threshold")
	timeRange := flagSet.String("time-range", defaultTimeRange, "Time range (HH:MM-HH:MM)")
	logLevel := flagSet.String("log-level", "INFO", "DEBUG|INFO|WARNING|ERROR")
	helpShort := flagSet.Bool("h", false, "Display usage information")
	help := flagSet.Bool("help", false, "Display usage information")

	if err := flagSet.Parse(args); err != nil {
		return config, false, err
	}
	if *help || *helpShort {
		printUsage(stdout)
		return config, true, nil
	}

	config.LatitudeLongitude = *latitudeLongitude
	config.Date = *date
	config.TemperatureThresholdDrift = *tempDrift
	config.PrecipitationThresholdPct = *precipPct
	config.PrecipitationThresholdMM = *precipMM
	config.TimeRange = *timeRange
	config.LogLevel = *logLevel
	config.DefaultApplied["latitude-longitude"] = !isFlagProvided(args, "latitude-longitude")
	config.DefaultApplied["date"] = !isFlagProvided(args, "date")
	config.DefaultApplied["temperature-threshold-drift"] = !isFlagProvided(args, "temperature-threshold-drift")
	config.DefaultApplied["precipitation-threshold_percent"] = !isFlagProvided(args, "precipitation-threshold_percent")
	config.DefaultApplied["precipitation-threshold_mm"] = !isFlagProvided(args, "precipitation-threshold_mm")
	config.DefaultApplied["time-range"] = !isFlagProvided(args, "time-range")
	config.DefaultApplied["log-level"] = !isFlagProvided(args, "log-level")

	return config, false, nil
}

func isFlagProvided(args []string, longName string) bool {
	full := "--" + longName
	withValue := full + "="
	for _, arg := range args {
		if arg == full || strings.HasPrefix(arg, withValue) {
			return true
		}
	}
	return false
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: weather_check [flags]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Flags:")
	fmt.Fprintln(out, "  --latitude-longitude               Location lat;lon (default: 40.3458;-3.8249)")
	fmt.Fprintln(out, "  --date                             Date in YYYY-MM-DD (default: next day)")
	fmt.Fprintln(out, "  --temperature-threshold-drift      Min temperature drift in C (default: 2)")
	fmt.Fprintln(out, "  --precipitation-threshold_percent  Min precipitation probability (default: 30)")
	fmt.Fprintln(out, "  --precipitation-threshold_mm       Min precipitation amount in mm (default: 2)")
	fmt.Fprintln(out, "  --time-range                       Time range HH:MM-HH:MM (default: 08:00-20:00)")
	fmt.Fprintln(out, "  --log-level                        DEBUG|INFO|WARNING|ERROR (default: INFO)")
	fmt.Fprintln(out, "  -h, --help                         Show this help")
}

func parseDateOrTomorrow(raw string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		nextDay := now.AddDate(0, 0, 1)
		return dateOnly(nextDay), nil
	}
	parsed, err := time.ParseInLocation(dateLayout, raw, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --date value %q (expected YYYY-MM-DD)", raw)
	}
	return dateOnly(parsed), nil
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func parseTimeRange(raw string) (TimeRange, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return TimeRange{}, fmt.Errorf("invalid --time-range %q (expected HH:MM-HH:MM)", raw)
	}
	startMinute, err := parseMinuteOfDay(strings.TrimSpace(parts[0]))
	if err != nil {
		return TimeRange{}, err
	}
	endMinute, err := parseMinuteOfDay(strings.TrimSpace(parts[1]))
	if err != nil {
		return TimeRange{}, err
	}
	if startMinute > endMinute {
		return TimeRange{}, fmt.Errorf("invalid --time-range %q (start after end)", raw)
	}
	return TimeRange{StartMinute: startMinute, EndMinute: endMinute}, nil
}

func parseMinuteOfDay(raw string) (int, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time %q", raw)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hour in %q", raw)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minute in %q", raw)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time out of range %q", raw)
	}
	return hour*60 + minute, nil
}

func minuteOfDay(value time.Time) int {
	return value.Hour()*60 + value.Minute()
}

func analyze(hourly []HourlyPoint, targetDate time.Time, timeRange TimeRange, tempDriftThreshold, precipPctThreshold, precipMMThreshold float64) (Report, error) {
	targetDate = dateOnly(targetDate)
	currentDate := targetDate.AddDate(0, 0, -1)

	targetRange := filterByDateAndRange(hourly, targetDate, timeRange)
	currentRange := filterByDateAndRange(hourly, currentDate, timeRange)

	if len(targetRange) == 0 {
		return Report{}, fmt.Errorf("no target date (%s) hourly records in selected range", targetDate.Format(dateLayout))
	}
	if len(currentRange) == 0 {
		return Report{}, fmt.Errorf("no current date (%s) hourly records in selected range", currentDate.Format(dateLayout))
	}

	targetMax, targetMin := maxMinTemp(targetRange)
	currentMax, currentMin := maxMinTemp(currentRange)

	maxChange := targetMax - currentMax
	minChange := targetMin - currentMin

	report := Report{
		TargetDate:       targetDate,
		DayName:          targetDate.Weekday().String(),
		MaxTempChange:    maxChange,
		MaxTempTarget:    targetMax,
		MinTempChange:    minChange,
		MinTempTarget:    targetMin,
		HasMaxTempChange: math.Abs(maxChange) >= tempDriftThreshold,
		HasMinTempChange: math.Abs(minChange) >= tempDriftThreshold,
	}

	for _, point := range targetRange {
		if point.PrecipitationProb >= precipPctThreshold && point.PrecipitationMM >= precipMMThreshold {
			report.SignificantRains = append(report.SignificantRains, point)
		}
	}

	sort.Slice(report.SignificantRains, func(i, j int) bool {
		return report.SignificantRains[i].Time.Before(report.SignificantRains[j].Time)
	})

	return report, nil
}

func filterByDateAndRange(points []HourlyPoint, day time.Time, timeRange TimeRange) []HourlyPoint {
	matched := make([]HourlyPoint, 0)
	for _, point := range points {
		timeInDay := minuteOfDay(point.Time)
		if sameCalendarDate(point.Time, day) && timeInDay >= timeRange.StartMinute && timeInDay <= timeRange.EndMinute {
			matched = append(matched, point)
		}
	}
	return matched
}

func sameCalendarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func maxMinTemp(points []HourlyPoint) (maxTemp float64, minTemp float64) {
	maxTemp = points[0].TemperatureC
	minTemp = points[0].TemperatureC
	for _, point := range points[1:] {
		if point.TemperatureC > maxTemp {
			maxTemp = point.TemperatureC
		}
		if point.TemperatureC < minTemp {
			minTemp = point.TemperatureC
		}
	}
	return maxTemp, minTemp
}

func renderReport(report Report) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("**%s Weather changes**", report.DayName))

	if report.HasMaxTempChange {
		lines = append(lines, fmt.Sprintf(". %s%sº max temp (%sºC)", signedNumber(report.MaxTempChange), degreeNumber(report.MaxTempChange), degreeNumber(report.MaxTempTarget)))
	}
	if report.HasMinTempChange {
		lines = append(lines, fmt.Sprintf(". %s%sº min temp (%sºC)", signedNumber(report.MinTempChange), degreeNumber(report.MinTempChange), degreeNumber(report.MinTempTarget)))
	}
	for _, rain := range report.SignificantRains {
		lines = append(lines, fmt.Sprintf(". %d:00 rain %smm (%s%%)", rain.Time.Hour(), compactNumber(rain.PrecipitationMM), compactNumber(rain.PrecipitationProb)))
	}

	return strings.Join(lines, "\n")
}

func signedNumber(value float64) string {
	if value >= 0 {
		return "+"
	}
	return "-"
}

func degreeNumber(value float64) string {
	return compactNumber(math.Abs(value))
}

func compactNumber(value float64) string {
	rounded := math.Round(value)
	if math.Abs(value-rounded) < 0.001 {
		return fmt.Sprintf("%.0f", rounded)
	}
	return fmt.Sprintf("%.1f", value)
}

func (s *OpenMeteoService) ResolveLocation(ctx context.Context, query string) (Location, error) {
	trimmed := strings.TrimSpace(query)
	if loc, ok := parseCoordinateLocation(trimmed); ok {
		if s.Logger != nil {
			s.Logger.Infof("Using coordinates from --latitude-longitude")
		}
		return loc, nil
	}

	params := url.Values{}
	params.Set("name", trimmed)
	params.Set("count", "1")
	params.Set("language", "en")
	params.Set("format", "json")
	requestURL := openMeteoGeoEndpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Location{}, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return Location{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Location{}, fmt.Errorf("geocoding request failed with status %s", resp.Status)
	}

	var payload struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Timezone  string  `json:"timezone"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Location{}, err
	}
	if len(payload.Results) == 0 {
		return Location{}, fmt.Errorf("no location matches for %q", query)
	}

	first := payload.Results[0]
	name := strings.TrimSpace(first.Name)
	if country := strings.TrimSpace(first.Country); country != "" {
		name = name + ", " + country
	}

	return Location{
		Name:      name,
		Latitude:  first.Latitude,
		Longitude: first.Longitude,
		Timezone:  first.Timezone,
	}, nil
}

func parseCoordinateLocation(raw string) (Location, bool) {
	parts := strings.Split(raw, ";")
	if len(parts) != 2 {
		return Location{}, false
	}
	lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errLat != nil || errLon != nil {
		return Location{}, false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return Location{}, false
	}
	return Location{Name: raw, Latitude: lat, Longitude: lon, Timezone: "auto"}, true
}

func (s *OpenMeteoService) HourlyForecast(ctx context.Context, loc Location, startDate time.Time, endDate time.Time) ([]HourlyPoint, error) {
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.6f", loc.Latitude))
	params.Set("longitude", fmt.Sprintf("%.6f", loc.Longitude))
	params.Set("hourly", "temperature_2m,precipitation_probability,precipitation")
	if strings.TrimSpace(loc.Timezone) != "" {
		params.Set("timezone", loc.Timezone)
	} else {
		params.Set("timezone", "auto")
	}
	params.Set("start_date", startDate.Format(dateLayout))
	params.Set("end_date", endDate.Format(dateLayout))

	requestURL := openMeteoForecastEndpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forecast request failed with status %s", resp.Status)
	}

	var payload struct {
		Hourly struct {
			Time              []string  `json:"time"`
			Temperature2M     []float64 `json:"temperature_2m"`
			PrecipitationProb []float64 `json:"precipitation_probability"`
			Precipitation     []float64 `json:"precipitation"`
		} `json:"hourly"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	count := len(payload.Hourly.Time)
	if count == 0 {
		return nil, errors.New("forecast payload had no hourly data")
	}
	if len(payload.Hourly.Temperature2M) != count || len(payload.Hourly.PrecipitationProb) != count || len(payload.Hourly.Precipitation) != count {
		return nil, errors.New("forecast payload has mismatched hourly arrays")
	}

	result := make([]HourlyPoint, 0, count)
	for idx := 0; idx < count; idx++ {
		parsedTime, err := time.Parse(dateTimeLayout, payload.Hourly.Time[idx])
		if err != nil {
			return nil, fmt.Errorf("invalid hourly time %q: %w", payload.Hourly.Time[idx], err)
		}
		result = append(result, HourlyPoint{
			Time:              parsedTime,
			TemperatureC:      payload.Hourly.Temperature2M[idx],
			PrecipitationProb: payload.Hourly.PrecipitationProb[idx],
			PrecipitationMM:   payload.Hourly.Precipitation[idx],
		})
	}

	return result, nil
}

func (s *OpenMeteoService) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}
