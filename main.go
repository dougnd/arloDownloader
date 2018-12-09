package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jeffreydwalter/arlo-golang"
	"github.com/pkg/errors"
)

var configFile = flag.String("config-file", "config.ini", "Configuration file name")

type config struct {
	Email    string
	Password string
	Days     int
}

func readConfig() config {
	_, err := os.Stat(*configFile)
	if err != nil {
		log.Fatal("Config file is missing: ", *configFile)
	}

	var c config
	if _, err := toml.DecodeFile(*configFile, &c); err != nil {
		log.Fatal(err)
	}
	return c
}

func downloadFile(url, to string) error {
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to do http get")
	}
	defer resp.Body.Close()

	f, err := os.Create(to)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to copy file")
	}

	return nil
}

func main() {

	c := readConfig()

	// Instantiating the Arlo object automatically calls Login(), which returns an oAuth token that gets cached.
	// Subsequent successful calls to login will update the oAuth token.
	arlo, err := arlo.Login(c.Email, c.Password)
	if err != nil {
		log.Printf("Failed to login: %s\n", err)
		return
	}
	// At this point you're logged into Arlo.

	now := time.Now()
	start := now.Add(time.Duration(c.Days) * 24 * time.Hour)

	// Get all of the recordings for a date range.
	library, err := arlo.GetLibrary(start, now)
	if err != nil {
		log.Println(err)
		return
	}

	for _, recording := range *library {

		filename := fmt.Sprintf("%s %s.mp4", time.Unix(0, recording.UtcCreatedDate*int64(time.Millisecond)).Format(("2006-01-02 15:04:05")), recording.UniqueId)
		outputPath := filepath.Join("videos", filename)
		_, err := os.Stat(outputPath)
		if err == nil {
			log.Printf("Skipping video already downloaded: %v", filename)
			continue
		}

		// The videos produced by Arlo are pretty small, even in their longest, best quality settings.
		// DownloadFile() efficiently streams the file from the http.Response.Body directly to a file.
		if err := downloadFile(recording.PresignedContentUrl, outputPath); err != nil {
			log.Println(err)
		} else {
			log.Printf("Downloaded video: %v\n", filename)
		}

	}

}
