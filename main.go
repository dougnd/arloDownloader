package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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

type worker struct {
	i  int
	ch chan arlo.Recording
	wg *sync.WaitGroup
}

func (w *worker) work() {
	log.Printf("Worker %v starting.", w.i)
	for r := range w.ch {
		filename := fmt.Sprintf("%s %s.mp4", time.Unix(0, r.UtcCreatedDate*int64(time.Millisecond)).Format(("2006-01-02 15:04:05")), r.UniqueId)
		outputPath := filepath.Join("videos", filename)
		_, err := os.Stat(outputPath)
		if err == nil {
			log.Printf("[W%v] Skipping video already downloaded: %v", w.i, filename)
			continue
		}

		if err := downloadFile(r.PresignedContentUrl, outputPath); err != nil {
			log.Println(err)
		} else {
			log.Printf("[W%v] Downloaded video: %v\n", w.i, filename)
		}
	}
	log.Printf("Worker %v done.", w.i)
	w.wg.Done()
}

const numWorkers = 5

func main() {

	c := readConfig()

	// Instantiating the Arlo object automatically calls Login(), which returns an oAuth token that gets cached.
	// Subsequent successful calls to login will update the oAuth token.
	a, err := arlo.Login(c.Email, c.Password)
	if err != nil {
		log.Printf("Failed to login: %s\n", err)
		return
	}
	// At this point you're logged into Arlo.

	recordingChan := make(chan arlo.Recording)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		w := worker{
			i:  i,
			wg: &wg,
			ch: recordingChan,
		}
		go w.work()
	}

	day0 := time.Now()
	for i := 0; i <= c.Days; i++ {
		d := day0.Add(-time.Duration(i) * 24 * time.Hour)
		log.Println("FETCHING for date %v", d)

		// Get all of the recordings for a date.
		library, err := a.GetLibrary(d, d)
		if err != nil {
			log.Println(err)
			return
		}

		for _, recording := range *library {
			recordingChan <- recording
		}
	}
	close(recordingChan)
	wg.Wait()

}
