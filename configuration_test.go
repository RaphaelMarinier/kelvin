package main

import (
	"strings"
	"testing"
	"time"
)

func TestReadOK(t *testing.T) {
	correctfiles := []string{
		"testdata/config-example.json",
		"testdata/config-example-newstyleschedule.json",
		"testdata/config-example.yaml",
	}
	for _, testFile := range correctfiles {
		c := Configuration{}
		c.ConfigurationFile = testFile
		err := c.Read()
		if err != nil {
			t.Fatalf("Could not read correct configuration file : %v with error : %v", c.ConfigurationFile, err)
		}
	}
}

type MockSunStateCalculator struct {
	MockSunrise time.Time
	MockSunset  time.Time
}

func (calculator *MockSunStateCalculator) CalculateSunset(date time.Time, latitude float64, longitude float64) time.Time {
	return calculator.MockSunset
}

func (calculator *MockSunStateCalculator) CalculateSunrise(date time.Time, latitude float64, longitude float64) time.Time {
	return calculator.MockSunrise
}

func parseTime(t string) time.Time {
	parsed, _ := time.Parse("2006-01-02 15:04", t)
	return parsed
}

func TestLightScheduleForDay(t *testing.T) {
	c := Configuration{}
	c.ConfigurationFile = "testdata/config-example-newstyleschedule.json"
	err := c.Read()
	if err != nil {
		t.Fatalf("Could not read correct configuration file : %v with error : %v", c.ConfigurationFile, err)
	}
	location := time.UTC
	calculator := &MockSunStateCalculator{
		time.Date(2021, 4, 28, 7, 30, 0, 0, location),
		time.Date(2021, 4, 28, 20, 0, 0, 0, location)}

	s, err := c.lightScheduleForDay(1, time.Date(2021, 4, 28, 0, 0, 1, 0, location), calculator)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}

	expectedTimes := []TimeStamp{
		TimeStamp{parseTime("2021-04-27 22:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 04:00"), 2000, 60},
		TimeStamp{parseTime("2021-04-28 07:30"), 2700, 60},
		TimeStamp{parseTime("2021-04-28 08:00"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 19:30"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 20:00"), 2700, 80},
		TimeStamp{parseTime("2021-04-28 22:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-29 04:00"), 2000, 60}}

	if len(s.Times) != len(expectedTimes) {
		t.Fatalf("Got schedule with unexpected length. Got %v expected %v", s.Times, expectedTimes)
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != s.Times[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v",
				i, s.Times[i], expectedTime)
		}
	}
}

func TestComputeNewStyleScheduleEasy(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 30m", ColorTemperature: 5000, Brightness: 100},
		{Time: "10:00", ColorTemperature: 6000, Brightness: 100},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 08:30")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 10:00"), 6000, 100},
		TimeStamp{parseTime("2021-04-28 08:00"), 2700, 80},
		// Sunrise.
		TimeStamp{parseTime("2021-04-28 08:30"), 3000, 90},
		// Sunrise + 30m.
		TimeStamp{parseTime("2021-04-28 09:00"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 10:00"), 6000, 100},
		// Next day.
		TimeStamp{parseTime("2021-04-29 08:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

func TestComputeNewStyleScheduleEasy2(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 30m", ColorTemperature: 5000, Brightness: 100},
		{Time: "sunset", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunset + 30m", ColorTemperature: 2000, Brightness: 80},
		{Time: "22:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 08:30")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 22:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 08:00"), 2700, 80},
		// Sunrise.
		TimeStamp{parseTime("2021-04-28 08:30"), 3000, 90},
		// Sunrise + 30m.
		TimeStamp{parseTime("2021-04-28 09:00"), 5000, 100},
		// Sunset
		TimeStamp{parseTime("2021-04-28 19:30"), 3000, 90},
		// Sunset + 30m
		TimeStamp{parseTime("2021-04-28 20:00"), 2000, 80},
		TimeStamp{parseTime("2021-04-28 22:00"), 2000, 70},
		// Next day
		TimeStamp{parseTime("2021-04-29 08:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

// Failing, as expected since the main code is not ready.
func TestComputeNewStyleScheduleClampedSunrise(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 30m", ColorTemperature: 5000, Brightness: 100},
		{Time: "22:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	// This is before the first time in the config.
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 22:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 08:00"), 2700, 80},
		// Clamped sunrise.
		TimeStamp{parseTime("2021-04-28 08:01"), 3000, 90},
		// Clamped sunrise + 30m.
		TimeStamp{parseTime("2021-04-28 08:31"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 22:00"), 2000, 70},
		// Next day
		TimeStamp{parseTime("2021-04-29 08:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

// Failing, as expected since the main code is not ready.
func TestComputeNewStyleScheduleClampedSunset(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 5000, Brightness: 100},
		{Time: "sunset", ColorTemperature: 4000, Brightness: 90},
		{Time: "sunset + 30m", ColorTemperature: 2000, Brightness: 90},
		{Time: "22:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	// This makes "sunset + 30m" be after the last time in the config.
	sunset := parseTime("2021-04-28 21:50")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 22:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 08:00"), 5000, 100},
		// Clamped sunset.
		TimeStamp{parseTime("2021-04-28 21:29"), 4000, 90},
		// Clamped sunset + 30m.
		TimeStamp{parseTime("2021-04-28 21:59"), 2000, 90},
		TimeStamp{parseTime("2021-04-28 22:00"), 2000, 70},
		// Next day
		TimeStamp{parseTime("2021-04-29 08:00"), 5000, 100},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

// Failing, as expected since the main code is not ready.
func TestComputeNewStyleScheduleImpossibleSunriseClamping(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 30m", ColorTemperature: 5000, Brightness: 100},
		{Time: "08:20", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err == nil {
		t.Fatalf("Expected error, got schedule %v", schedule)
	}
}

// Failing, as expected since the main code is not ready.
func TestComputeNewStyleScheduleImpossibleSunsetClamping(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "20:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunset", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunset + 30m", ColorTemperature: 5000, Brightness: 100},
		{Time: "20:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err == nil {
		t.Fatalf("Expected error, got schedule %v", schedule)
	}
}

func TestComputeNewStyleScheduleComplexClamping1(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 180m", ColorTemperature: 5000, Brightness: 100},
		{Time: "sunset - 180m", ColorTemperature: 4000, Brightness: 100},
		{Time: "sunset + 180m", ColorTemperature: 3000, Brightness: 100},
		{Time: "18:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	// This is before the first time in the config.
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 19:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 18:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 08:00"), 2700, 80},
		// Sunrise (clamped to be after 8:00).
		TimeStamp{parseTime("2021-04-28 08:01"), 3000, 90},
		// Clamped sunrise + 180m.
		TimeStamp{parseTime("2021-04-28 11:01"), 5000, 100},
		// clamped sunset - 180m
		TimeStamp{parseTime("2021-04-28 11:59"), 4000, 100},
		// clamped sunset + 180m
		TimeStamp{parseTime("2021-04-28 17:59"), 3000, 100},
		TimeStamp{parseTime("2021-04-28 18:00"), 2000, 70},
		// Next day
		TimeStamp{parseTime("2021-04-29 08:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

func TestComputeNewStyleScheduleComplexClamping2(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 180m", ColorTemperature: 5000, Brightness: 100},
		{Time: "sunset - 240m", ColorTemperature: 4000, Brightness: 100},
		{Time: "sunset + 180m", ColorTemperature: 3000, Brightness: 100},
		{Time: "19:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	// This is before the first time in the config.
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 14:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 19:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 08:00"), 2700, 80},
		// Sunrise (clamped to be after 8:00).
		TimeStamp{parseTime("2021-04-28 08:01"), 3000, 90},
		// Clamped sunrise + 180m.
		TimeStamp{parseTime("2021-04-28 11:01"), 5000, 100},
		// clamped sunset - 180m
		TimeStamp{parseTime("2021-04-28 11:02"), 4000, 100},
		// clamped sunset + 180m
		TimeStamp{parseTime("2021-04-28 18:02"), 3000, 100},
		TimeStamp{parseTime("2021-04-28 19:00"), 2000, 70},
		// Next day
		TimeStamp{parseTime("2021-04-29 08:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}
}

func TestComputeNewStyleScheduleImpossible1(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "8:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise - 240m", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 240m", ColorTemperature: 5000, Brightness: 100},
		{Time: "15:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 14:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if !strings.Contains(err.Error(), "cannot be satisfied") {
		t.Fatalf("Got unexpected error %v and schedule %v", err, schedule)
	}
}

func TestComputeNewStyleScheduleImpossible2(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "10:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunset - 240m", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunset + 240m", ColorTemperature: 5000, Brightness: 100},
		{Time: "17:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 14:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if !strings.Contains(err.Error(), "cannot be satisfied") {
		t.Fatalf("Got unexpected error %v and schedule %v", err, schedule)
	}
}

func TestComputeNewStyleScheduleImpossible3(t *testing.T) {
	configSchedule := []TimedColorTemperature{
		{Time: "10:00", ColorTemperature: 2700, Brightness: 80},
		{Time: "sunrise - 120m", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunrise + 120m", ColorTemperature: 5000, Brightness: 100},
                {Time: "sunset - 120m", ColorTemperature: 3000, Brightness: 90},
		{Time: "sunset + 120m", ColorTemperature: 5000, Brightness: 100},
		{Time: "17:00", ColorTemperature: 2000, Brightness: 70},
	}
	date := parseTime("2021-04-28 00:01")
	sunrise := parseTime("2021-04-28 07:00")
	sunset := parseTime("2021-04-28 14:30")
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if !strings.Contains(err.Error(), "cannot be satisfied") {
		t.Fatalf("Got unexpected error %v and schedule %v", err, schedule)
	}
}

func TestReadError(t *testing.T) {
	wrongfiles := []string{
		"",          // no file passed
		"testdata/", // not a regular file
		"testdata/config-bad-wrongFormat.json",
		"testdata/config-bad-wrongFormat.yaml",
	}
	for _, testFile := range wrongfiles {
		c := Configuration{}
		c.ConfigurationFile = testFile
		err := c.Read()
		if err == nil {
			t.Errorf("reading [%v] file should return an error", c.ConfigurationFile)
		}
	}
}

func TestWriteOK(t *testing.T) {
	correctfiles := []string{
		"testdata/config-example.json",
		"testdata/config-example.yaml",
	}
	for _, testFile := range correctfiles {
		c := Configuration{}
		c.ConfigurationFile = testFile
		_ = c.Read()
		c.Hash = ""
		err := c.Write()
		if err != nil {
			t.Errorf("Could not write configuration to correct file : %v", c.ConfigurationFile)
		}
	}
}
