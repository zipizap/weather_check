package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type fakeWeatherService struct {
	location Location
	hourly   []HourlyPoint
}

func (f fakeWeatherService) ResolveLocation(ctx context.Context, query string) (Location, error) {
	return f.location, nil
}

func (f fakeWeatherService) HourlyForecast(ctx context.Context, loc Location, startDate time.Time, endDate time.Time) ([]HourlyPoint, error) {
	return f.hourly, nil
}

func TestRun_PrintsExpectedSignificantChanges(t *testing.T) {
	targetDate := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	currentDate := targetDate.AddDate(0, 0, -1)

	data := []HourlyPoint{
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 15, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 20, PrecipitationProb: 20, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 16, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 10, PrecipitationProb: 40, PrecipitationMM: 2},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 9, 0, 0, 0, time.UTC), TemperatureC: 12, PrecipitationProb: 35, PrecipitationMM: 3},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 27, PrecipitationProb: 15, PrecipitationMM: 0},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 11, PrecipitationProb: 5, PrecipitationMM: 0},
	}

	service := fakeWeatherService{
		location: Location{Name: "Alcorcon, Spain", Latitude: 40.35, Longitude: -3.82, Timezone: "Europe/Madrid"},
		hourly:   data,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{
		"--latitude-longitude", "40.3458;-3.8249",
		"--date", "2026-02-16",
		"--temperature-threshold-drift", "4",
		"--precipitation-threshold_percent", "30",
		"--precipitation-threshold_mm", "2",
		"--time-range", "08:00-20:00",
		"--log-level", "INFO",
	}, &stdout, &stderr, service)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "**Monday Weather changes**") {
		t.Fatalf("expected weekday title in output, got: %s", output)
	}
	if !strings.Contains(output, "- +7º max temp (27ºC)") {
		t.Fatalf("expected max temperature change line in output, got: %s", output)
	}
	if !strings.Contains(output, "- -5º min temp (10ºC)") {
		t.Fatalf("expected min temperature change line in output, got: %s", output)
	}
	if !strings.Contains(output, "- 8:00 rain 2mm (40%)") || !strings.Contains(output, "- 9:00 rain 3mm (35%)") {
		t.Fatalf("expected significant rain lines in output, got: %s", output)
	}
}

func TestRun_NoSignificantChanges(t *testing.T) {
	targetDate := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
	currentDate := targetDate.AddDate(0, 0, -1)

	data := []HourlyPoint{
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 12, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 18, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 13, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 12.5, PrecipitationProb: 20, PrecipitationMM: 1},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 18.3, PrecipitationProb: 25, PrecipitationMM: 1.5},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 13.2, PrecipitationProb: 20, PrecipitationMM: 1},
	}

	service := fakeWeatherService{
		location: Location{Name: "Alcorcon, Spain", Latitude: 40.35, Longitude: -3.82, Timezone: "Europe/Madrid"},
		hourly:   data,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{
		"--latitude-longitude", "40.3458;-3.8249",
		"--date", "2026-02-17",
		"--temperature-threshold-drift", "4",
		"--precipitation-threshold_percent", "30",
		"--precipitation-threshold_mm", "2",
		"--time-range", "08:00-20:00",
		"--log-level", "INFO",
	}, &stdout, &stderr, service)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	output := stdout.String()
	if strings.TrimSpace(output) != "" {
		t.Fatalf("expected no stdout output when there are no significant changes, got: %s", output)
	}
}

func TestRun_ShortHelpFlag(t *testing.T) {
	service := fakeWeatherService{}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{"-h"}, &stdout, &stderr, service)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Usage: weather_check [flags]") {
		t.Fatalf("expected usage output, got: %s", output)
	}
	if !strings.Contains(output, "-h, --help") {
		t.Fatalf("expected short help line in usage output, got: %s", output)
	}
}

func TestParseCoordinateLocation_RequiresSemicolon(t *testing.T) {
	if _, ok := parseCoordinateLocation("40.3458,-3.8249"); ok {
		t.Fatal("expected comma format to be rejected")
	}
	if _, ok := parseCoordinateLocation("40.3458;-3.8249"); !ok {
		t.Fatal("expected semicolon format to be accepted")
	}
}

func TestRun_LogsArgsAndDefaultsAtStart(t *testing.T) {
	targetDate := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	currentDate := targetDate.AddDate(0, 0, -1)

	data := []HourlyPoint{
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 12, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 18, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 13, PrecipitationProb: 10, PrecipitationMM: 0},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 8, 0, 0, 0, time.UTC), TemperatureC: 12.5, PrecipitationProb: 20, PrecipitationMM: 1},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 12, 0, 0, 0, time.UTC), TemperatureC: 18.3, PrecipitationProb: 25, PrecipitationMM: 1.5},
		{Time: time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 20, 0, 0, 0, time.UTC), TemperatureC: 13.2, PrecipitationProb: 20, PrecipitationMM: 1},
	}

	service := fakeWeatherService{
		location: Location{Name: "Coords", Latitude: 40.3458, Longitude: -3.8249, Timezone: "Europe/Madrid"},
		hourly:   data,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{
		"--latitude-longitude", "40.3458;-3.8249",
		"--date", "2026-02-18",
	}, &stdout, &stderr, service)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	logs := stderr.String()
	if !strings.Contains(logs, "Arg --latitude-longitude=\"40.3458;-3.8249\" (user)") {
		t.Fatalf("expected latitude-longitude startup log, got: %s", logs)
	}
	if !strings.Contains(logs, "Arg --temperature-threshold-drift=\"2\" (default)") {
		t.Fatalf("expected defaulted threshold startup log, got: %s", logs)
	}
	if !strings.Contains(logs, "Defaults applied:") {
		t.Fatalf("expected defaults summary in logs, got: %s", logs)
	}
}
