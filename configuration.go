// MIT License
//
// Copyright (c) 2018 Stefan Wichmann
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
)

// Bridge respresents the hue bridge in your system.
type Bridge struct {
	IP       string `json:"ip"`
	Username string `json:"username"`
}

// Location represents the geolocation for which sunrise and sunset will be calculated.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// WebInterface respresents the webinterface of Kelvin.
type WebInterface struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

// TODO: update webinterface.go with the new-style schedule.

// LightSchedule represents the schedule for any given day for the associated lights.
type LightSchedule struct {
	Name                   string `json:"name"`
	AssociatedDeviceIDs    []int  `json:"associatedDeviceIDs"`
	EnableWhenLightsAppear bool   `json:"enableWhenLightsAppear"`

	// Old-style schedule. Not used when the new-style schedule below is used.
	DefaultColorTemperature int                     `json:"defaultColorTemperature"`
	DefaultBrightness       int                     `json:"defaultBrightness"`
	BeforeSunrise           []TimedColorTemperature `json:"beforeSunrise"`
	AfterSunset             []TimedColorTemperature `json:"afterSunset"`

	// New-style schedule.
	// The `time` field of each time point can be a time (HH:MM), 'sunrise', 'sunset',
	// 'sunrise +- NN minutes', 'sunset +- NN minutes'.
	Schedule []TimedColorTemperature `json:"schedule"`
}

// Type of a time point, i.e. whether it comes from a fixed time (e.g. "12:00"), a
// sunrise specification (e.g. "sunrise - 10m") or a sunset specification
// (e.g. "sunset + 10m")
type TimePointType int

const (
	UnsetTimePoint    TimePointType = iota
	FixedTimePoint    TimePointType = iota
	Sunrise           TimePointType = iota
	Sunset            TimePointType = iota
	NumTimePointTypes TimePointType = iota
)

// TimedColorTemperature represents a light configuration which will be
// reached at the given time.
type TimedColorTemperature struct {
	Time             string `json:"time"`
	ColorTemperature int    `json:"colorTemperature"`
	Brightness       int    `json:"brightness"`

	// Result from parsing "Time".
	ParsedTimePointType TimePointType `json:"-"`
	// Only specified when ParsedTimePointType == FixedTimePoint.
	ParsedTimeInDay time.Time `json:"-"`
	// Only specified when ParsedTimePointType is Sunrise or Sunset.
	ParsedOffset time.Duration `json:"-"`
}

// Configuration encapsulates all relevant parameters for Kelvin to operate.
type Configuration struct {
	ConfigurationFile string          `json:"-"`
	Hash              string          `json:"-"`
	Version           int             `json:"version"`
	Bridge            Bridge          `json:"bridge"`
	Location          Location        `json:"location"`
	WebInterface      WebInterface    `json:"webinterface"`
	Schedules         []LightSchedule `json:"schedules"`
}

// TimeStamp represents a parsed and validated TimedColorTemperature.
type TimeStamp struct {
// TODO: add unparsed field for pretty-printing (e.g. in dashboard).
	Time             time.Time
	ColorTemperature int
	Brightness       int
}

var latestConfigurationVersion = 0

func (configuration *Configuration) initializeDefaults() {
	configuration.Version = latestConfigurationVersion

	var defaultSchedule LightSchedule
	defaultSchedule.Name = "default"
	defaultSchedule.AssociatedDeviceIDs = []int{}
	// TODO: is this still used?
	defaultSchedule.DefaultColorTemperature = 2750
        // TODO: is this still used?
	defaultSchedule.DefaultBrightness = 100
	defaultSchedule.Schedule = []TimedColorTemperature{	
		TimedColorTemperature{Time: "sunrise - 1h", ColorTemperature: 2000, Brightness: 50},
		TimedColorTemperature{Time: "sunrise - 10m", ColorTemperature: 2700, Brightness: 80},
		TimedColorTemperature{Time: "sunrise + 10m", ColorTemperature: 5000, Brightness: 100},
		TimedColorTemperature{Time: "16:00", ColorTemperature: 5000, Brightness: 100},
		TimedColorTemperature{Time: "sunset - 30m", ColorTemperature: 3000, Brightness: 100},
		TimedColorTemperature{Time: "sunset", ColorTemperature: 2700, Brightness: 100},
		TimedColorTemperature{Time: "22:00", ColorTemperature: 2000, Brightness: 70},
	}

	configuration.Schedules = []LightSchedule{defaultSchedule}

	var webinterface WebInterface
	webinterface.Enabled = false
	webinterface.Port = 8080
	configuration.WebInterface = webinterface
}

// InitializeConfiguration creates and returns an initialized
// configuration.
// If no configuration can be found on disk, one with default values
// will be created.
func InitializeConfiguration(configurationFile string, enableWebInterface bool) (Configuration, error) {
	var configuration Configuration
	configuration.ConfigurationFile = configurationFile
	if configuration.Exists() {
		err := configuration.Read()
		if err != nil {
			return configuration, err
		}
		log.Printf("⚙ Configuration %v loaded", configuration.ConfigurationFile)
	} else {
		// write default config to disk
		configuration.initializeDefaults()
		err := configuration.Write()
		if err != nil {
			return configuration, err
		}
		log.Println("⚙ Default configuration generated")
	}

	// Overwrite interface configuration with startup parameter
	if enableWebInterface {
		configuration.WebInterface.Enabled = true
		err := configuration.Write()
		if err != nil {
			return configuration, err
		}
	}
	return configuration, nil
}

// Write saves a configuration to disk.
func (configuration *Configuration) Write() error {
	if configuration.ConfigurationFile == "" {
		return errors.New("No configuration filename configured")
	}

	if !configuration.HasChanged() {
		log.Debugf("⚙ Configuration hasn't changed. Omitting write.")
		return nil
	}
	log.Debugf("⚙ Configuration changed. Saving to %v", configuration.ConfigurationFile)
	raw, err := json.MarshalIndent(configuration, "", "  ")
	if err != nil {
		return err
	}

	// Convert JSON to YAML if needed
	if isYAMLFile(configuration.ConfigurationFile) {
		raw, err = yaml.JSONToYAML(raw)
		if err != nil {
			return err
		}
	}

	err = ioutil.WriteFile(configuration.ConfigurationFile, raw, 0644)
	if err != nil {
		return err
	}

	configuration.Hash = configuration.HashValue()
	log.Debugf("⚙ Updated configuration hash")
	return nil
}

// Read loads a configuration from disk.
func (configuration *Configuration) Read() error {
	if configuration.ConfigurationFile == "" {
		return errors.New("No configuration filename configured")
	}

	raw, err := ioutil.ReadFile(configuration.ConfigurationFile)
	if err != nil {
		return err
	}

	// Convert YAML to JSON if needed
	if isYAMLFile(configuration.ConfigurationFile) {
		raw, err = yaml.YAMLToJSON(raw)
		if err != nil {
			return err
		}
	}

	err = json.Unmarshal(raw, configuration)
	if err != nil {
		return err
	}

	if len(configuration.Schedules) == 0 {
		log.Warningf("⚙ Your current configuration doesn't contain any schedules! Generating default schedule...")
		err := configuration.backup()
		if err != nil {
			log.Warningf("⚙ Could not create backup: %v", err)
		} else {
			log.Printf("⚙ Configuration backup created.")
			configuration.initializeDefaults()
			log.Printf("⚙ Default schedule created.")
			configuration.Write()
		}
	}
	configuration.Hash = configuration.HashValue()
	log.Debugf("⚙ Updated configuration hash.")

	configuration.migrateToLatestVersion()
	configuration.Write()
	return nil
}

func ComputeNewStyleSchedule(configSchedule []TimedColorTemperature,
	sunrise time.Time, sunset time.Time, date time.Time) ([]TimeStamp, error) {
	log.Warningf("⚙ computeNewStyleSchedule")
	yr, mth, dy := date.Date()
	startOfDay := time.Date(yr, mth, dy, 0, 0, 0, 0, date.Location())
	endOfDay := time.Date(yr, mth, dy, 23, 59, 59, 0, date.Location())
	var timeStamps []TimeStamp
	for i, _ := range configSchedule {
		err := configSchedule[i].ParseTime()
		if err != nil {
			return timeStamps, err
		}
	}

	// Dummy TimedColorTemperature to start the day. This is used
	// to clamp the first times of the day (corner case where
	// somebody writes "sunrise - large value", not to determine the
	// light temperature or brightness).
	previousConfig := &TimedColorTemperature{"", -1, -1, FixedTimePoint,
		startOfDay, time.Duration(0)}
	// realSun contains real sunrise/sunset times for the current day.
        // adjustedSun will contain adjusted sunrise/sunset so that a sunrise- or
	// sunset-based time never crosses a fixed time.
	var adjustedSun, realSun [NumTimePointTypes]time.Time
	realSun[Sunset] = sunset
	realSun[Sunrise] = sunrise
	adjustedSun = realSun
	// First pass where we adjust the sunrise and sunset to later times if needed.
	log.Warningf("⚙ Processing schedule %+v", configSchedule)
	for i, _ := range configSchedule {
		if i-1 >= 0 {
			previousConfig = &configSchedule[i-1]
		}
		previousTime := previousConfig.AsTime(startOfDay, adjustedSun[Sunrise], adjustedSun[Sunset])
		currentConfig := &configSchedule[i]
		currentTime := currentConfig.AsTime(startOfDay, adjustedSun[Sunrise], adjustedSun[Sunset])
		log.Warningf("⚙ Processing %+v (%+v) %+v (%+v)", previousConfig, previousTime, currentConfig, currentTime)
		if currentTime.After(previousTime) || currentTime.Equal(previousTime) {
			continue
		}
		log.Warningf("⚙ Inversion %+v %+v", previousConfig, currentConfig)
		// currentTime is before previousTime, we need to adjust things when possible.
		if previousConfig.ParsedTimePointType == FixedTimePoint && currentConfig.ParsedTimePointType == FixedTimePoint {
			return timeStamps, fmt.Errorf("Wrong order in schedule: '%v' appeared before '%v'", previousConfig.Time, currentConfig.Time)
		}
		if previousConfig.ParsedTimePointType != FixedTimePoint && currentConfig.ParsedTimePointType != FixedTimePoint {
			// Inversion of two consecutive non-fixed time points.
			// We only allow this when the first is sunrise-based and the second is sunset-based.
			// This disallows mis-ordered time specs such as {"sunrise", "sunrise-10m"} or sunset appearing before sunrise.
			if previousConfig.ParsedTimePointType != Sunrise || currentConfig.ParsedTimePointType != Sunset {
				return timeStamps, fmt.Errorf("Wrong order in schedule: '%v' appeared before '%v'", previousConfig.Time, currentConfig.Time)
			}
		}
		if currentConfig.ParsedTimePointType != FixedTimePoint {
			// Adjust currentConfig by moving the (potentially already adjusted) sunset or
			// sunrise to a later time.
			offset := previousTime.Sub(currentTime) // Positive duration.
			adjustedSun[currentConfig.ParsedTimePointType] = adjustedSun[currentConfig.ParsedTimePointType].Add(offset)
			// One minute transition.
			adjustedSun[currentConfig.ParsedTimePointType] = adjustedSun[currentConfig.ParsedTimePointType].Add(time.Minute)
			log.Warningf("⚙ Adjusting sun %v to %+v (real %+v)", currentConfig.ParsedTimePointType, adjustedSun[currentConfig.ParsedTimePointType], realSun[currentConfig.ParsedTimePointType])
		}
	}

	// Second pass (from later time points to earlier in the day) where we adjust sunrise
	// and sunset to earlier times if needed.
	// Dummy fixed time point to end the day. Only used to clamp sunrise/sunset, not for the color
	// temperature nor brightness.
	nextConfig := &TimedColorTemperature{"", -1, -1, FixedTimePoint,
		endOfDay, time.Duration(0)}
	for i := len(configSchedule) - 1; i >= 0; i-- {
		if i+1 < len(configSchedule) {
			nextConfig = &configSchedule[i+1]
		}
		nextTime := nextConfig.AsTime(startOfDay, adjustedSun[Sunrise], adjustedSun[Sunset])
		currentConfig := &configSchedule[i]
		currentTime := currentConfig.AsTime(startOfDay, adjustedSun[Sunrise], adjustedSun[Sunset])
		if currentTime.Before(nextTime) || currentTime.Equal(nextTime) {
			continue
		}
		// We need to adjust the sunset/sunrise to an earlier time.
		if currentConfig.ParsedTimePointType != FixedTimePoint {
			offset := nextTime.Sub(currentTime) // Negative duration
			adjustedSun[currentConfig.ParsedTimePointType] = adjustedSun[currentConfig.ParsedTimePointType].Add(offset)
			// One minute transition.
			adjustedSun[currentConfig.ParsedTimePointType] = adjustedSun[currentConfig.ParsedTimePointType].Add(-time.Minute)
		}
	}

	// Now, build the TimeStamps, check that the schedule is consistent, otherwise,
	// return an error as it it no satisfiable.
	// First, add the last time point from the previous day, to make sure we fully cover
	// the current day.
	// To guarantee that, we clamp the time from the last config in the
	// previous day to one minute before midnight the current day (in some corner
	// cases, or with "sunset + large value"), the last value of the previous day could
	// end up after midnight.
	lastConfig := configSchedule[len(configSchedule)-1]
	startOfPreviousDay := startOfDay.AddDate(0, 0, -1)
	previousDaySunrise := sunrise.AddDate(0, 0, -1)
	previousDaySunset := sunset.AddDate(0, 0, -1)
	firstTimeStamp := TimeStamp{lastConfig.AsTime(startOfPreviousDay, previousDaySunrise, previousDaySunset),
		lastConfig.ColorTemperature, lastConfig.Brightness}
	// TODO: check if the 1 minute is really useful (and if it is, fix the condition which is
	// not full correct)
	if firstTimeStamp.Time.After(startOfDay) || firstTimeStamp.Time.Equal(startOfDay) {
		// TODO: log a warning.
		firstTimeStamp.Time = startOfDay.Add(-time.Minute)
	}
	timeStamps = append(timeStamps, firstTimeStamp)
	for _, config := range configSchedule {
		timeStamps = append(timeStamps,
			TimeStamp{config.AsTime(startOfDay, adjustedSun[Sunrise], adjustedSun[Sunset]),
				config.ColorTemperature, config.Brightness})
	}
	// Add first timestamp of the next day to make sure we cover the current day fully.
	// Similarly to the last timestamp of the previous day, we clamp at midnight.
	firstConfig := configSchedule[0]
	startOfNextDay := startOfDay.AddDate(0, 0, 1)
	// Approximations, probably good enough.
	nextDaySunrise := sunrise.AddDate(0, 0, 1)
	nextDaySunset := sunset.AddDate(0, 0, 1)
	lastTimeStamp := TimeStamp{firstConfig.AsTime(startOfNextDay, nextDaySunrise, nextDaySunset),
		firstConfig.ColorTemperature, firstConfig.Brightness}
	if lastTimeStamp.Time.Before(startOfNextDay) {
		// TODO: log a warning.
		// Do we need one more minute?
		lastTimeStamp.Time = startOfNextDay
	}
	timeStamps = append(timeStamps, lastTimeStamp)

	// Check that there is no inversion left, otherwise, it means that schedule
	// cannot be satisfied, even when moving sunrise/sunset.
	for i, _ := range timeStamps {
		if i+1 >= len(timeStamps) {
			break
		}
		if timeStamps[i].Time.After(timeStamps[i+1].Time) {
			// Note difference of one in timeStamps and configSchedule indices.
			curConfig := configSchedule[(i+len(configSchedule)-1)%len(configSchedule)]
			nextConfig := configSchedule[i%len(configSchedule)]
			return timeStamps, fmt.Errorf(
				"Schedule cannot be satisfied. With real sunrise %v, adjusted sunrise: %v, real sunset: %v, adjusted sunset: %v, we still get %v (%v) was still after %v (%v)",
				realSun[Sunrise], adjustedSun[Sunrise], realSun[Sunset], adjustedSun[Sunset], curConfig.Time, timeStamps[i].Time, nextConfig, timeStamps[i+1].Time)
		}
	}
	return timeStamps, nil
}

func (configuration *Configuration) lightScheduleForDay(
	light int, date time.Time, sunStateCalculator SunStateCalculatorInterface) (Schedule, error) {
	// initialize schedule with end of day
	var schedule Schedule
	yr, mth, dy := date.Date()
	schedule.endOfDay = time.Date(yr, mth, dy, 23, 59, 59, 59, date.Location())

	var lightSchedule LightSchedule
	found := false
	for _, candidate := range configuration.Schedules {
		if containsInt(candidate.AssociatedDeviceIDs, light) {
			lightSchedule = candidate
			found = true
			break
		}
	}

	// TODO: is there a check that a light is not associated with multiple schedules?
	if !found {
		return schedule, fmt.Errorf("Light %d is not associated with any schedule in configuration", light)
	}

	schedule.enableWhenLightsAppear = lightSchedule.EnableWhenLightsAppear
	schedule.sunrise = TimeStamp{sunStateCalculator.CalculateSunrise(date, configuration.Location.Latitude, configuration.Location.Longitude), lightSchedule.DefaultColorTemperature, lightSchedule.DefaultBrightness}
	schedule.sunset = TimeStamp{sunStateCalculator.CalculateSunset(date, configuration.Location.Latitude, configuration.Location.Longitude), lightSchedule.DefaultColorTemperature, lightSchedule.DefaultBrightness}

	if len(lightSchedule.Schedule) > 0 {
		// New-style schedules in the config. When present, we
		// populate the new-style schedule `schedule.Times`.
		newScheduleTimes, err := ComputeNewStyleSchedule(lightSchedule.Schedule, schedule.sunrise.Time, schedule.sunset.Time, date)
		if err != nil {
			return schedule, err
		}
		schedule.Times = newScheduleTimes
		return schedule, nil
	}

	// Old-style schedule.
	// Before sunrise candidates
	schedule.beforeSunrise = []TimeStamp{}
	for _, candidate := range lightSchedule.BeforeSunrise {
		timestamp, err := candidate.AsTimestamp(date)
		if err != nil {
			log.Warningf("⚙ Found invalid configuration entry before sunrise: %+v (Error: %v)", candidate, err)
			continue
		}
		schedule.beforeSunrise = append(schedule.beforeSunrise, timestamp)
	}

	// After sunset candidates
	schedule.afterSunset = []TimeStamp{}
	for _, candidate := range lightSchedule.AfterSunset {
		timestamp, err := candidate.AsTimestamp(date)
		if err != nil {
			log.Warningf("⚙ Found invalid configuration entry after sunset: %+v (Error: %v)", candidate, err)
			continue
		}
		schedule.afterSunset = append(schedule.afterSunset, timestamp)
	}

	return schedule, nil
}

// Exists return true if a configuration file is found on disk.
// False otherwise.
func (configuration *Configuration) Exists() bool {
	if configuration.ConfigurationFile == "" {
		return false
	}

	if _, err := os.Stat(configuration.ConfigurationFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// HasChanged will detect changes to the configuration struct.
func (configuration *Configuration) HasChanged() bool {
	if configuration.Hash == "" {
		return true
	}
	return configuration.HashValue() != configuration.Hash
}

// HashValue will calculate a SHA256 hash of the configuration struct.
func (configuration *Configuration) HashValue() string {
	json, _ := json.Marshal(configuration)
	return fmt.Sprintf("%x", sha256.Sum256(json))
}

// AsTimestamp parses and validates a TimedColorTemperature and returns
// a corresponding TimeStamp.
func (color *TimedColorTemperature) AsTimestamp(referenceTime time.Time) (TimeStamp, error) {
	layout := "15:04"
	t, err := time.Parse(layout, color.Time)
	if err != nil {
		return TimeStamp{time.Now(), color.ColorTemperature, color.Brightness}, err
	}
	yr, mth, day := referenceTime.Date()
	targetTime := time.Date(yr, mth, day, t.Hour(), t.Minute(), t.Second(), 0, referenceTime.Location())

	return TimeStamp{targetTime, color.ColorTemperature, color.Brightness}, nil
}

// This function parses the time field of a TimedColorTemperature coming from the config.
// Accepted formats:
// - HH:MM
// - (sunrise|sunset) [ (+|-) NN m[inutes] ]
// with obvious semantics.
func (color *TimedColorTemperature) ParseTime() error {
	re := regexp.MustCompile(`(?P<time>\d{1,2}:\d\d)|(?P<spec>(sunrise|sunset)(\s*(\+|-)\s*(\d+)\s*m.*){0,1})`)
	matches := re.FindStringSubmatch(color.Time)
        log.Warningf("⚙ Matches: %+v", matches) // TODO: bug probably comes from the submatch logic (sunrise - 1h gets matched as 'sunrise').
	if len(matches[0]) == 0 {
		return fmt.Errorf("Invalid timestamp %v", color.Time)
	}
	if len(matches[1]) > 0 {
		// Time of the form hh:mm
		layout := "15:04"
		t, err := time.Parse(layout, color.Time)
		if err != nil {
			return fmt.Errorf("Failed to parse %v as a HH:MM time: %v", color.Time, err)
		}
		color.ParsedTimePointType = FixedTimePoint
		color.ParsedTimeInDay = t
		return nil
	} else if len(matches[2]) > 0 {
		// sunrise|sunset [(+|-) NN minutes].
		if matches[3] == "sunrise" {
			color.ParsedTimePointType = Sunrise
		} else { // sunset
			color.ParsedTimePointType = Sunset
		}
		if len(matches[4]) > 0 { // Offset to the sunrise/sunset.
			minutes, err := strconv.Atoi(matches[6])
			if err != nil {
				return fmt.Errorf("Failed to parse sunrise/sunset offset %v: %v", matches[6], err)
			}
			if matches[5] == "+" {
				color.ParsedOffset = time.Minute * time.Duration(minutes)
			} else {
				// minus
				color.ParsedOffset = -time.Minute * time.Duration(minutes)
			}
		}
		return nil
	}
	return fmt.Errorf("Internal error parsing time %v", color.Time)
}

// Given a TimedColorTemperature on which ParseTime() has been called (otherwise, we panic()),
// returns the corresponding time.Time.
func (color *TimedColorTemperature) AsTime(startOfDay time.Time, sunrise time.Time, sunset time.Time) time.Time {
	switch color.ParsedTimePointType {
	case FixedTimePoint:
		{
			yr, mth, dy := startOfDay.Date()
			return time.Date(yr, mth, dy, color.ParsedTimeInDay.Hour(),
				color.ParsedTimeInDay.Minute(), 0, 0, startOfDay.Location())
			//, nil
		}
	case Sunrise:
		return sunrise.Add(color.ParsedOffset) //, nil
	case Sunset:
		return sunset.Add(color.ParsedOffset) //, nil
	default:
		panic(fmt.Errorf("Internal error: TimedColorTemperature.ParseTime was not called %v", color))
	}
}

func (configuration *Configuration) backup() error {
	backupFilename := configuration.ConfigurationFile + "_" + time.Now().Format("01022006")
	log.Debugf("⚙ Moving configuration to %s.", backupFilename)
	return os.Rename(configuration.ConfigurationFile, backupFilename)
}
