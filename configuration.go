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

// LightSchedule represents the schedule for any given day for the associated lights.
type LightSchedule struct {
	Name                   string `json:"name"`
	AssociatedDeviceIDs    []int  `json:"associatedDeviceIDs"`
	EnableWhenLightsAppear bool   `json:"enableWhenLightsAppear"`
	// Remove?
	DefaultColorTemperature int `json:"defaultColorTemperature"`
	// Remove?
	DefaultBrightness int `json:"defaultBrightness"`
	// Remove?
	BeforeSunrise []TimedColorTemperature `json:"beforeSunrise"`
	// Remove?
	AfterSunset []TimedColorTemperature `json:"afterSunset"`
	// The time in json can be a time (HH:MM), sunrise, sunset, sunrise + NN minutes, sunset + NN minutes
	// There could be a "clamps sunset/sunrise option" (default on).
	Schedule []TimedColorTemperature `json:"schedule"`
}

// TimedColorTemperature represents a light configuration which will be
// reached at the given time.
type TimedColorTemperature struct {
	Time             string `json:"time"`
	ColorTemperature int    `json:"colorTemperature"`
	Brightness       int    `json:"brightness"`
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
	Time             time.Time
	ColorTemperature int
	Brightness       int
}

var latestConfigurationVersion = 0

func (configuration *Configuration) initializeDefaults() {
	configuration.Version = latestConfigurationVersion

	var bedTime TimedColorTemperature
	bedTime.Time = "22:00"
	bedTime.ColorTemperature = 2000
	bedTime.Brightness = 60

	var tvTime TimedColorTemperature
	tvTime.Time = "20:00"
	tvTime.ColorTemperature = 2300
	tvTime.Brightness = 80

	var wakeupTime TimedColorTemperature
	wakeupTime.Time = "4:00"
	wakeupTime.ColorTemperature = 2000
	wakeupTime.Brightness = 60

	var defaultSchedule LightSchedule
	defaultSchedule.Name = "default"
	defaultSchedule.AssociatedDeviceIDs = []int{}
	defaultSchedule.DefaultColorTemperature = 2750
	defaultSchedule.DefaultBrightness = 100
	defaultSchedule.AfterSunset = []TimedColorTemperature{tvTime, bedTime}
	defaultSchedule.BeforeSunrise = []TimedColorTemperature{wakeupTime}

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

// TODO.
func (configuration *Configuration) lightScheduleForDay(light int, date time.Time) (Schedule, error) {
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

	schedule.sunrise = TimeStamp{CalculateSunrise(date, configuration.Location.Latitude, configuration.Location.Longitude), lightSchedule.DefaultColorTemperature, lightSchedule.DefaultBrightness}
	schedule.sunset = TimeStamp{CalculateSunset(date, configuration.Location.Latitude, configuration.Location.Longitude), lightSchedule.DefaultColorTemperature, lightSchedule.DefaultBrightness}

	if len(lightSchedule.Schedule) > 0 {
		// New-style schedules in the json config. When present, we populare the new-style schedule `schedule.times`.
		// Add the last time point from the previous day.
		previous_day_last_timestamp, err := lightSchedule.Schedule[len(lightSchedule.Schedule)-1].AsTimestamp2(date.AddDate(0, 0, -1), schedule.sunrise.Time, schedule.sunset.Time)
		if err != nil {
			log.Warningf("⚙ Found invalid configuration entry in schedule: %+v (Error: %v)", lightSchedule.Schedule[len(lightSchedule.Schedule)-1], err)
			// TODO
		}
		log.Warningf("Last timepoint %v", previous_day_last_timestamp)

		schedule.times = append(schedule.times, previous_day_last_timestamp)
		for _, timedColorTemp := range lightSchedule.Schedule {
			// TODO: add last elemt of the previous day as first item in schedule.times.
			timestamp, err := timedColorTemp.AsTimestamp2(date, schedule.sunrise.Time, schedule.sunset.Time)
			if err != nil {
				log.Warningf("⚙ Found invalid configuration entry in schedule: %+v (Error: %v)", timedColorTemp, err)
				continue
			}
			// TODO: if timestamp.Time is <= previous time point, fix it.
			previousTime := schedule.times[len(schedule.times)-1].Time
			// TODO: double-check condition,
			if len(schedule.times) > 0 && timestamp.Time.Before(previousTime) {
				// TODO: make it an error when the time inversion is due to static times.
				log.Warningf("Found time inversion %v is before %v", timestamp.Time, previousTime)
				timestamp.Time = previousTime.Add(time.Minute)
			}
			log.Warningf("Adding timepoint %v", timestamp)
			schedule.times = append(schedule.times, timestamp)
			// TODO: for last elmt, add one from the next day.
		}
		next_day_first_timestamp, err := lightSchedule.Schedule[0].AsTimestamp2(date.AddDate(0, 0, 1), schedule.sunrise.Time, schedule.sunset.Time)
		if err != nil {
			log.Warningf("⚙ Found invalid configuration entry in schedule: %+v (Error: %v)", lightSchedule.Schedule[0], err)
			// TODO
		}
		log.Warningf("First timepoint next day %v", next_day_first_timestamp)
		schedule.times = append(schedule.times, next_day_first_timestamp)
	}

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

	schedule.enableWhenLightsAppear = lightSchedule.EnableWhenLightsAppear
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

// referenceTime is an arbitrary time in the current day.
func (color *TimedColorTemperature) AsTimestamp2(referenceTime time.Time, sunrise time.Time, sunset time.Time) (TimeStamp, error) {
	re := regexp.MustCompile(`(?P<time>\d{1,2}:\d\d)|(?P<spec>(sunrise|sunset)(\s*(\+|-)\s*(\d+)\s*m.*){0,1})`)
	//	if err != nil {
	//		return TimeStamp{time.Now(), color.ColorTemperature, color.Brightness}, err
	//        }
	matches := re.FindStringSubmatch(color.Time)
	if len(matches[0]) == 0 {
		return TimeStamp{time.Now(), color.ColorTemperature, color.Brightness}, fmt.Errorf("Invalid timestamp %v", color.Time)
	}
	var ret TimeStamp
	if len(matches[1]) > 0 {
		// Time of the form hh:mm
		layout := "15:04"
		t, err := time.Parse(layout, color.Time)
		if err != nil {
			return TimeStamp{time.Now(), color.ColorTemperature, color.Brightness}, err
		}
		yr, mth, day := referenceTime.Date()
		ret.Time = time.Date(yr, mth, day, t.Hour(), t.Minute(), t.Second(), 0, referenceTime.Location())
	} else if len(matches[2]) > 0 {
		// sunrise|sunset [(+|-) NN minutes].
		if matches[3] == "sunrise" {
			ret.Time = sunrise
		} else {
			ret.Time = sunset
		}
		if len(matches[4]) > 0 {
			minutes, err := strconv.Atoi(matches[6])
			if err != nil {
				return TimeStamp{time.Now(), color.ColorTemperature, color.Brightness}, err
			}
			if matches[5] == "+" {
				ret.Time = ret.Time.Add(time.Minute * time.Duration(minutes))
			} else {
				// minus
				ret.Time = ret.Time.Add(-time.Minute * time.Duration(minutes))
			}
		}
	}
	ret.ColorTemperature = color.ColorTemperature
	ret.Brightness = color.Brightness
	return ret, nil
}

func (configuration *Configuration) backup() error {
	backupFilename := configuration.ConfigurationFile + "_" + time.Now().Format("01022006")
	log.Debugf("⚙ Moving configuration to %s.", backupFilename)
	return os.Rename(configuration.ConfigurationFile, backupFilename)
}
