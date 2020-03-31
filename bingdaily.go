package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path"
	"sort"
	"strings"
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
	log.Println("Starting bingdaily run")

	hd, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to locate homedir: %w", err)
	}

	targetDir := path.Join(hd, imgDir)

	log.Printf("Attempt to create output directory: %s\n", targetDir)

	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to make image directory: %w", err)
	}

	im, err := getLatestMetadata()
	if err != nil {
		return fmt.Errorf("failed to obtain metadata: %w", err)
	}

	log.Printf("Obtained image metadata: %v\n", im)

	err = downloadImage(targetDir, im)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}

	log.Println("Updating background image")

	err = setWallpaper(targetDir)
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

func downloadImage(imageDir string, im *imageMetadata) error {
	filename := path.Join(imageDir, im.Hash+".jpg")

	log.Printf("Checking for existence of file: %s", filename)

	exists, err := imageExists(filename)
	if err != nil {
		return fmt.Errorf("unable to determine whether file exists: %v", err)
	}

	if exists {
		log.Println("Image already exists, no need to download")
		return nil
	}

	fullURL := bingRoot + im.URL

	resp, err := http.Get(fullURL)
	if err != nil {
		return fmt.Errorf("error while downloading image: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code: %s", resp.Status)
	}

	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write image to output file: %w", err)
	}

	return nil
}

func imageExists(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func setWallpaper(dirname string) error {
	err := removeOldFiles(dirname)
	if err != nil {
		return fmt.Errorf("Failed to remove old files: %v", err)
	}

	filename, err := chooseImage(dirname)
	if err != nil {
		return fmt.Errorf("Failed to choose an image")
	}

	dbusAddress, err := obtainDbusAddress()
	if err != nil {
		return fmt.Errorf("Failed to obtain dbus address: %w", err)
	}

	fullFilename := "file://" + path.Join(dirname, filename)

	log.Printf("Full filename: %s\n", fullFilename)

	cmd := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", fullFilename)

	env := os.Environ()
	env = append(env, dbusAddress[:len(dbusAddress)-1])
	cmd.Env = env // ensure we forward the environment to the new shell including the dbus address

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to set wallpaper: %w\n%s", err, out.String())
	}
	return nil
}

type fileSlice []os.FileInfo

func (f fileSlice) Len() int           { return len(f) }
func (f fileSlice) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f fileSlice) Less(i, j int) bool { return f[i].ModTime().Before(f[j].ModTime()) }

func removeOldFiles(dirname string) error {
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		return fmt.Errorf("Failed to read image directory: %v", err)
	}

	if len(files) < 10 {
		log.Println("No images to delete")
		return nil
	}

	sort.Sort(fileSlice(files))

	filesToDelete := files[0 : len(files)-10]

	for _, file := range filesToDelete {
		filename := path.Join(dirname, file.Name())
		log.Printf("Deleting image: %s\n", filename)

		err = os.Remove(filename)
		if err != nil {
			return fmt.Errorf("Failed to delete image: %v", err)
		}
	}

	return nil
}

func chooseImage(dirname string) (string, error) {
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		return "", fmt.Errorf("Failed to read image directory: %v", err)
	}

	rand.Seed(time.Now().Unix())

	return files[rand.Intn(len(files))].Name(), nil
}

func obtainDbusAddress() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("Failed to obtain current user: %v", err)
	}

	var out bytes.Buffer

	cmd := exec.Command("pgrep", "--euid", currentUser.Uid, "gnome-session")
	cmd.Stdout = &out

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("Failed to obtain gnome-session PID: %v", err)
	}

	pid := strings.TrimSpace(out.String())
	out.Reset()

	cmd = exec.Command("grep", "-z", "DBUS_SESSION_BUS_ADDRESS", fmt.Sprintf("/proc/%s/environ", pid))
	cmd.Stdout = &out

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("Failed to obtain dbus address: %v", err)
	}

	return out.String(), nil
}
