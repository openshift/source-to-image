package util

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
)

type Downloader interface {
	DownloadFile(url *url.URL, targetFile string) error
}

// schemeReader creates an io.Reader from the given url.
type schemeReader interface {
	Read(*url.URL) (io.ReadCloser, error)
}

type HttpURLReader struct {
	httpGet func(url string) (*http.Response, error)
}

type FileURLReader struct{}

type downloader struct {
	verbose       bool
	schemeReaders map[string]schemeReader
}

func NewDownloader(verbose bool) Downloader {
	httpReader := NewHttpReader()
	return &downloader{
		verbose: verbose,
		schemeReaders: map[string]schemeReader{
			"http":  httpReader,
			"https": httpReader,
			"file":  &FileURLReader{},
		},
	}
}

func NewHttpReader() schemeReader {
	r := &HttpURLReader{}
	r.httpGet = http.Get
	return r
}

// readerFromHttpUrl produces an io.Reader from an http(s) URL.
func (h *HttpURLReader) Read(url *url.URL) (io.ReadCloser, error) {
	resp, err := h.httpGet(url.String())
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
		}
		return nil, err
	}
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		return resp.Body, nil
	} else {
		return nil, fmt.Errorf("Failed to retrieve %s, response code %d", url.String(), resp.StatusCode)
	}
}

// readerFromFileUrl produces an io.Redader from a file URL
func (f *FileURLReader) Read(url *url.URL) (io.ReadCloser, error) {
	return os.Open(url.Path)
}

// DownloadFile downloads the file pointed to by URL into local targetFile
func (d *downloader) DownloadFile(url *url.URL, targetFile string) error {
	sr := d.schemeReaders[url.Scheme]

	if sr == nil {
		log.Printf("ERROR: No URL handler found for url %s", url.String())
		return fmt.Errorf("ERROR: No URL handler found for url %s", url.String())
	}
	reader, err := sr.Read(url)
	if err != nil {
		if d.verbose {
			log.Printf("ERROR: Unable to download %s (%s)\n", url.String(), err)
		}
		return err
	}
	defer reader.Close()

	out, err := os.Create(targetFile)
	defer out.Close()

	if err != nil {
		defer os.Remove(targetFile)
		log.Printf("ERROR: Unable to create target file %s (%s)\n", targetFile, err)
		return err
	}

	_, err = io.Copy(out, reader)

	if err != nil {
		defer os.Remove(targetFile)
		log.Printf("Skipping file %s due to error copying from source: %s\n", targetFile, err)
	}

	if d.verbose {
		log.Printf("Downloaded '%s'\n", url.String())
	}
	return nil
}
