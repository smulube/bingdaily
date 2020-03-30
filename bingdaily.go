package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"

	homedir "github.com/mitchellh/go-homedir"
)

const (
	bingRoot = "https://www.bing.com"
	bingURL  = bingRoot + "/HPImageArchive.aspx?format=js&idx=0&n=1"
	imgDir   = ".bingdaily"
)

type imageMetadata struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Hash  string `json:"hsh"`
}

// response is a type used for parsing the response from bing
type response struct {
	Images []imageMetadata `json:"images"`
}

func main() {
	err := Execute()
	if err != nil {
		log.Fatal(err)
	}
}

// Execute is where we actually do our work
func Execute() error {
	hd, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to locate homedir: %w", err)
	}

	targetDir := path.Join(hd, imgDir)

	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to make image directory: %w", err)
	}

	im, err := getLatestMetadata()
	if err != nil {
		return fmt.Errorf("failed to obtain metadata: %w", err)
	}

	imageName, err := downloadImage(targetDir, im)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	err = setWallpaper(imageName)
	if err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	return nil
}

func getLatestMetadata() (*imageMetadata, error) {
	var r response
	resp, err := http.Get(bingURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download metadata: %w", err)
	}

	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	if len(r.Images) == 0 {
		return nil, errors.New("no images found in JSON response")
	}

	return &r.Images[0], nil
}

func downloadImage(imageDir string, im *imageMetadata) (string, error) {
	filename := path.Join(imageDir, time.Now().Format("2006-01-02")) + ".jpg"

	fullURL := bingRoot + im.URL

	resp, err := http.Get(fullURL)
	if err != nil {
		return "", fmt.Errorf("error while downloading image: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected response code: %s", resp.Status)
	}

	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}

	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write image to output file: %w", err)
	}

	return filename, nil
}

func setWallpaper(filename string) error {
	fullFilename := "file://" + filename
	cmd := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", fullFilename)
	cmd.Env = os.Environ() // ensure we forward the environment to the new shell
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to set wallpaper: %w\n%s", err, out.String())
	}
	return nil
}
