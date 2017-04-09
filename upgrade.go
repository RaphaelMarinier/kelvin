// MIT License
//
// Copyright (c) 2017 Stefan Wichmann
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

import log "github.com/Sirupsen/logrus"
import "runtime"
import "os"
import "os/exec"
import "path/filepath"
import "github.com/hashicorp/go-version"
import "time"

const upgradeURL = "https://api.github.com/repos/stefanwichmann/kelvin/releases/latest"
const updateCheckIntervalInMinutes = 24 * 60

// CheckForUpdate will get the latest release information of Kelvin
// from github and compare it to the given version. If a newer version
// is found it will try to replace the running binary and restart.
func CheckForUpdate(currentVersion string) {
	// only look for update if version string matches a valid release version
	version, err := version.NewVersion(currentVersion)
	if err != nil {
		return
	}

	for {
		log.Printf("Looking for updates...")
		avail, url, err := updateAvailable(version, upgradeURL)
		if err != nil {
			log.Warningf("Error looking for update: %v", err)
		} else if avail {
			err = updateBinary(url)
			if err != nil {
				log.Warningf("Error updating binary: %v.", err)
			} else {
				log.Printf("Restarting...")
				Restart()
			}
		}
		// try again in 24 hours...
		time.Sleep(updateCheckIntervalInMinutes * time.Minute)
	}
}

// Restart the running binary.
// All arguments, pipes and environment variables will
// be preserved.
func Restart() {
	binary := os.Args[0]
	args := []string{}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	cmd.Start()
	os.Exit(0)
}

func updateAvailable(currentVersion *version.Version, url string) (bool, string, error) {
	releaseName, assetURL, err := downloadLatestReleaseInfo(url)
	if err != nil {
		return false, "", err
	}

	// parse name and compare
	version, err := version.NewVersion(releaseName)
	if err != nil {
		log.Debugf("Could not parse release name: %v", err)
		return false, "", err
	}

	if version.GreaterThan(currentVersion) {
		log.Printf("Found new release version %s.", version)
		return true, assetURL, nil
	}

	return false, "", nil
}

func updateBinary(assetURL string) error {
	currentBinary := os.Args[0]
	log.Printf("Downloading update archive %s", assetURL)
	archive, err := downloadReleaseArchive(assetURL)
	if err != nil {
		os.Remove(archive)
		return err
	}
	defer os.Remove(archive)
	log.Debugf("Update archive downloaded to %v", archive)

	// Find and extract binary
	var tempBinary string
	defer os.Remove(tempBinary)
	if runtime.GOOS == "windows" {
		tempBinary, err = extractBinaryFromZipArchive(archive, currentBinary, filepath.Dir(os.Args[0]))
		if err != nil {
			return err
		}
	} else {
		tempBinary, err = extractBinaryFromTarArchive(archive, currentBinary, filepath.Dir(os.Args[0]))
		if err != nil {
			return err
		}
	}

	// make binary executable
	err = os.Chmod(tempBinary, os.FileMode(0755))
	if err != nil {
		return err
	}

	// Replace binary
	log.Debugf("Replacing current binary %v with %v", currentBinary, tempBinary)
	err = replaceBinary(currentBinary, tempBinary)
	if err != nil {
		return err
	}

	log.Printf("Update successful")
	return nil
}

func replaceBinary(binaryFile, tempFile string) error {
	old := binaryFile + ".old"
	os.Remove(old) // remove old backup
	err := os.Rename(binaryFile, old)
	if err != nil {
		return err
	}
	if os.Rename(tempFile, binaryFile); err != nil {
		os.Rename(old, binaryFile)
		return err
	}
	return nil
}
