package util

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/golang/glog"
)

// Downloader downloads the specified URL to the target file location
type Downloader interface {
	DownloadFile(url *url.URL, targetFile string) (bool, error)
}

// schemeReader creates an io.Reader from the given url.
type schemeReader interface {
	IsFromImage() bool
	Read(*url.URL) (io.ReadCloser, error)
}

// HttpURLReader retrieves a response from a given http(s) URL
type HttpURLReader struct {
	httpGet func(url string) (*http.Response, error)
}

// FileURLReader opens a specified file and returns its stream
type FileURLReader struct{}

// ImageReader just returns information the URL is from inside the image
type ImageReader struct{}

type downloader struct {
	schemeReaders map[string]schemeReader
}

// NewDownloader creates an instance of the default Downloader implementation
func NewDownloader() Downloader {
	httpReader := NewHttpReader()
	return &downloader{
		schemeReaders: map[string]schemeReader{
			"http":  httpReader,
			"https": httpReader,
			"file":  &FileURLReader{},
			"image": &ImageReader{},
		},
	}
}

// NewHttpReader creates an instance of the HttpURLReader
func NewHttpReader() schemeReader {
	r := &HttpURLReader{}
	r.httpGet = http.Get
	return r
}

// IsFromImage returns information whether URL is from inside the image
func (h *HttpURLReader) IsFromImage() bool {
	return false
}

// Read produces an io.Reader from an http(s) URL.
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

// IsFromImage returns information whether URL is from inside the image
func (h *FileURLReader) IsFromImage() bool {
	return false
}

// Read produces an io.Reader from a file URL
func (f *FileURLReader) Read(url *url.URL) (io.ReadCloser, error) {
	return os.Open(url.Path)
}

// IsFromImage returns information whether URL is from inside the image
func (h *ImageReader) IsFromImage() bool {
	return true
}

// Read throws ZeroError to notify the input is of length 0.
func (f *ImageReader) Read(url *url.URL) (io.ReadCloser, error) {
	return nil, errors.New("Not implemented")
}

// DownloadFile downloads the file pointed to by URL into local targetFile
// Returns information a boolean flag informing whether any download/copy operation
// happened and an error if there was a problem during that operation
func (d *downloader) DownloadFile(url *url.URL, targetFile string) (bool, error) {
	sr := d.schemeReaders[url.Scheme]

	if sr == nil {
		glog.Errorf("No URL handler found for %s", url.String())
		return false, fmt.Errorf("No URL handler found url %s", url.String())
	}
	if sr.IsFromImage() {
		glog.V(2).Infof("Using image internal scripts from: %s", url.String())
		return false, nil
	}
	reader, err := sr.Read(url)
	if err != nil {
		glog.V(2).Infof("Unable to download %s (%s)", url.String(), err)
		return false, err
	}
	defer reader.Close()

	out, err := os.Create(targetFile)
	defer out.Close()

	if err != nil {
		defer os.Remove(targetFile)
		glog.Errorf("Unable to create target file %s (%s)", targetFile, err)
		return false, err
	}

	if _, err = io.Copy(out, reader); err != nil {
		defer os.Remove(targetFile)
		glog.Warningf("Skipping file %s due to error copying from source: %s", targetFile, err)
	}

	glog.V(2).Infof("Downloaded '%s'", url.String())
	return true, nil
}
