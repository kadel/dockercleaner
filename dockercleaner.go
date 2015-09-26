package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/samalba/dockerclient"
)

var (
	cleanOld  string
	cleanNone bool
	stopOld   string
	dockerURL string
	noConfirm bool
)

func init() {
	flag.StringVar(&dockerURL, "docker", "unix:///var/run/docker.sock", "Connection to Docker.")
	flag.StringVar(&cleanOld, "clean-old", "", "Delete all images older than this. Use units: 'h','m','s'")
	flag.BoolVar(&cleanNone, "clean-none", false, "Delete all untagged images. (<none>:<none>)")
	flag.StringVar(&stopOld, "stop-old", "", "Stop all container stat are running longer then this. Use units: 'h','m','s'")
	flag.BoolVar(&noConfirm, "yes", false, "Do not require confirmation.")
}

func stopCotainers(docker *dockerclient.DockerClient, imageIDs []string) []error {
	var errors []error
	if !noConfirm {
		var answer string
		fmt.Printf("This will stop %v images\n", len(imageIDs))
		fmt.Printf("Do you want to continue? (yes/no)\n")
		fmt.Scanln(&answer)
		if answer != "yes" {
			fmt.Printf("%v", answer)
			return nil
		}
	}

	for _, imageID := range imageIDs {
		stopErr := docker.StopContainer(imageID, 10)
		if stopErr != nil {
			errors = append(errors, stopErr)
		} else {
			log.Printf("STOPPED: %v", imageID)
		}
	}
	if len(errors) > 0 {
		return errors
	}
	return nil
}

func deleteImage(docker *dockerclient.DockerClient, imageIDs []string) []error {
	var errors []error
	if !noConfirm {
		var answer string
		fmt.Printf("This will delete %v images\n", len(imageIDs))
		fmt.Printf("Do you want to continue? (yes/no)\n")
		fmt.Scanln(&answer)
		if answer != "yes" {
			fmt.Printf("%v", answer)
			return nil
		}
	}

	for _, imageID := range imageIDs {
		imageDelete, deleteErr := docker.RemoveImage(imageID)
		if deleteErr != nil {
			errors = append(errors, deleteErr)
		}
		for _, id := range imageDelete {
			if id.Deleted != "" {
				log.Printf("DELETED: %v", id.Deleted)
			}
			if id.Untagged != "" {
				log.Printf("UNTAGGED: %#v", id.Untagged)
			}
		}
	}
	if len(errors) > 0 {
		return errors
	}
	return nil
}

func main() {
	flag.Parse()

	// check if there is an action
	if !cleanNone && cleanOld == "" && stopOld == "" {
		flag.Usage()
		return
	}

	var (
		toDelete []string
		toStop   []string
	)

	docker, dockerErr := dockerclient.NewDockerClient(dockerURL, nil)
	if dockerErr != nil {
		log.Fatalf("%v\n", dockerErr)
	}

	// Use system time from docker daemon for computing how old image is
	info, infoErr := docker.Info()
	if infoErr != nil {
		log.Fatalf("%v\n", infoErr)
	}
	systemTime := info.SystemTime.Unix()

	// Stop old containers
	if stopOld != "" {
		durationRunning, durationRunningErr := time.ParseDuration(stopOld)
		if durationRunningErr != nil {
			log.Fatalf("%v", durationRunningErr)
		}
		log.Printf("Stoppping containers that are running longer than: %v", durationRunning)

		running, runningErr := docker.ListContainers(false, false, "")
		if runningErr != nil {
			log.Fatalf("%v", runningErr)
		}
		for _, c := range running {
			containerAge := systemTime - c.Created
			log.Printf("%v", containerAge)
			if containerAge > int64(durationRunning.Seconds()) {
				log.Printf("Going to stop container %v, %v", c.Id, c.Status)
				toStop = append(toStop, c.Id)
			}
		}

		stopErrs := stopCotainers(docker, toStop)
		if stopErrs != nil {
			for _, stopErr := range stopErrs {
				log.Printf("%v", stopErr)
			}
		}
	}

	var images []*dockerclient.Image
	var errorList error
	// Get image list only when cleaning images
	if cleanOld != "" || cleanNone {
		images, errorList = docker.ListImages()
		if errorList != nil {
			log.Fatalf("%v", errorList)
		}
	}
	// Old images
	if cleanOld != "" {
		duration, durationErr := time.ParseDuration(cleanOld)
		if durationErr != nil {
			log.Fatalf("%v", durationErr)
		}

		for _, image := range images {
			imageAge := systemTime - image.Created
			if imageAge > int64(duration.Seconds()) {
				log.Printf("Going to delete %v (%v) because image is older then  %v", image.Id, image.RepoTags, duration)
				toDelete = append(toDelete, image.Id)
			}
		}
	}

	// Untaged images
	if cleanNone {
		for _, image := range images {
			if len(image.RepoTags) == 1 && image.RepoTags[0] == "<none>:<none>" {
				log.Printf("Going to delete %v (%v) becouse it is not tagged", image.Id, image.RepoTags)
				toDelete = append(toDelete, image.Id)
			}
		}
	}

	// Delete images
	if cleanOld != "" || cleanNone {
		deleteErrs := deleteImage(docker, toDelete)
		if deleteErrs != nil {
			for _, deleteErr := range deleteErrs {
				log.Printf("%v", deleteErr)
			}
		}
	}
}
