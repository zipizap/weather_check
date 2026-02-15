# weather_check

CLI tool to detect meaningful weather changes for a target day using online forecast data.

It compares a target date against the previous day (within a configurable time window), then reports:
- significant max/min temperature drift,
- significant hourly rain events (probability + amount).

If there are no significant results, it prints nothing to stdout.

## Features

- Fetches hourly forecast data from Open-Meteo.
- Uses coordinates input (`latitude;longitude`).
- Compares target day vs previous day in a time range (default `08:00-20:00`).
- Prints a concise human-readable report to stdout only when there are findings.
- Logs execution steps and intermediate values to stderr with log levels and colors.
- Logs startup arguments and whether each value came from user input or defaults.

## Requirements

- Go 1.22+
- Internet access (for weather/geocoding API calls)

## Quick start

```bash
go run .
```

Or with explicit values:

```bash
go run . \
  --latitude-longitude "40.3458;-3.8249" \
  --date "2026-02-17" \
  --temperature-threshold-drift 2 \
  --precipitation-threshold_percent 30 \
  --precipitation-threshold_mm 2 \
  --time-range "08:00-20:00" \
  --log-level INFO
```

## Flags

- `--latitude-longitude` (default: `40.3458;-3.8249`) location in format `latitude;longitude`
- `--date` (default: next day) target date in `YYYY-MM-DD`
- `--temperature-threshold-drift` (default: `2`) minimum significant temperature change (°C)
- `--precipitation-threshold_percent` (default: `30`) minimum rain probability (%)
- `--precipitation-threshold_mm` (default: `2`) minimum rain amount (mm)
- `--time-range` (default: `08:00-20:00`) analysis window `HH:MM-HH:MM`
- `--log-level` (default: `INFO`) `DEBUG|INFO|WARNING|ERROR`
- `-h, --help` show usage

## Output behavior

Example stdout when there are significant changes:

```text
**Monday Weather changes**
- +5º max temp (27ºC)
- -7º min temp (12ºC)
- 8:00 rain 2mm (40%)
- 9:00 rain 3mm (35%)
```

When there are no significant changes, stdout is empty.

## Logging

- Logs go to stderr.
- Includes startup argument values and whether they were user-provided or defaulted.
- Uses colored level labels (`DEBUG`, `INFO`, `WARNING`, `ERROR`).

## Testing

Run all tests:

```bash
go test ./...
```

## Project files

- `main.go` main CLI and weather logic
- `main_test.go` unit tests (mock weather service)
- `DESIGN.md` design/specification notes
