package main

import (
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
	parsed, _ := time.Parse("2006-01-02 15:04:05", t)
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
		TimeStamp{parseTime("2021-04-27 22:00:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-28 04:00:00"), 2000, 60},
		TimeStamp{parseTime("2021-04-28 07:30:00"), 2700, 60},
		TimeStamp{parseTime("2021-04-28 08:00:00"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 19:30:00"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 20:00:00"), 2700, 80},
		TimeStamp{parseTime("2021-04-28 22:00:00"), 2000, 70},
		TimeStamp{parseTime("2021-04-29 04:00:00"), 2000, 60}}

	if len(s.times) != len(expectedTimes) {
		t.Fatalf("Got schedule with unexpected length. Got %v expected %v", s.times, expectedTimes)
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != s.times[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v",
				i, s.times[i], expectedTime)
		}
	}
}

func TestComputeNewStyleScheduleEasy(t *testing.T) {
	// TODO.
	configSchedule := []TimedColorTemperature{
		{"8:00", 2700, 80},
		{"sunrise", 3000, 90},
		{"sunrise + 30m", 5000, 100},
		{"10:00", 6000, 100},
	}
	location := time.UTC
	date := time.Date(2021, 4, 28, 0, 0, 1, 0, location)
	sunrise := time.Date(2021, 4, 28, 8, 30, 0, 0, location)
	sunset := time.Date(2021, 4, 28, 19, 30, 0, 0, location)
	schedule, err := ComputeNewStyleSchedule(configSchedule, sunrise, sunset, date)
	if err != nil {
		t.Fatalf("Got error %v", err)
	}
	expectedTimes := []TimeStamp{
		// Previous day.
		TimeStamp{parseTime("2021-04-27 10:00:00"), 6000, 100},
		TimeStamp{parseTime("2021-04-28 08:00:00"), 2700, 80},
		// Sunrise.
		TimeStamp{parseTime("2021-04-28 08:30:00"), 3000, 90},
		// Sunrise + 30m.
		TimeStamp{parseTime("2021-04-28 09:00:00"), 5000, 100},
		TimeStamp{parseTime("2021-04-28 10:00:00"), 6000, 100},
		// Next day.
		TimeStamp{parseTime("2021-04-29 08:00:00"), 2700, 80},
	}
	for i, expectedTime := range expectedTimes {
		if expectedTime != schedule[i] {
			t.Fatalf("Got unexpected timestamp at position %v. Got %v expected %v.\nFull schedule obtained: %v, full schedule expected: %v",
				i, schedule[i], expectedTime, schedule, expectedTimes)
		}
	}

}

// TODO: test case when sunrise moves around, same for sunset.
// TODO: add logging.

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
