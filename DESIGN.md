# Design

## Specification

Use online meteoroligical services to get the hourly forecast of temperature and precipitation probability for the next day (0:00 to 23:59)

Compare if there is a significant probability of precipitation and ammount (e.g., >= 30% and >= 2mm) from 08:00 to 20:00
Compare if there is a significant drop-or-increase in max-min-temperature (e.g., >= or <= 2°C) from 08:00 to 20:00 compared to the current day max-min-temperature.

Finally if there are results, show them in a human readable format, to stdout, like this: 
```
**Monday Weather changes**
- +5º max temp (27ºC)
- -7º min temp (12ºC)
- 8:00 rain 2mm (40%)
- 9:00 rain 3mm (35%)
```
If there are no results, dont output anything.




### Arguments and flags
- `--latitude-longitude` (required): The location for which to check the weather, in the format "latitude;longitude". Defaults to `40.3458;-3.8249`.
- `--date` (optional): The date for which to check the weather, in YYYY-MM-DD format. Defaults to the next day.
- `--temperature-threshold-drift` (optional): The minimum temperature change (in degrees Celsius) to consider as significant. Defaults to `2` meaning 2°C.
- `--precipitation-threshold_percent` (optional): The minimum precipitation probability (in percentage) to consider as significant. Defaults to `30` meaning 30%.
- `--precipitation-threshold_mm` (optional): The minimum precipitation amount (in millimeters) to consider as significant. Defaults to `2` meaning 2mm.
- `--time-range` (optional): The time range to check for significant weather changes, in the format "HH:MM-HH:MM". Defaults to "08:00-20:00".

### Tests

You can use a mock weather service to return predefined weather data for testing purposes.



## Additional design specs

### Logging

During execution, log the steps and intermediate results and any errors, to stderr, with appropriate log levels (e.g., INFO, WARNING, ERROR) and colors. 
At start, include in the logs the values of the arguments used for the execution, and any defaults applied (except sensitive values, that should be redacted)

### Args and flags

- `--log-level`: Set the logging level (e.g., DEBUG, INFO, WARNING, ERROR). Defaults to INFO.
- `-h, --help`: Display usage information and exit.

### Tests

Add tests with conveninent values for the arguments, and check the output and errors are appropriate. 