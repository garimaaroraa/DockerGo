package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	imageToPull := os.Args[2]
	if !strings.Contains(imageToPull, "/") {
		imageToPull = "library/" + imageToPull
	}
	authToken := getAuthToken(imageToPull)
	layers := getManifest(imageToPull, authToken)

	command := os.Args[3]

	args := os.Args[4:len(os.Args)]
	file, err := ioutil.TempDir("/tmp", "jail")
	if err != nil {
		fmt.Printf("Err: %v", err)
		os.Exit(1)
	}

	pullLayers(file, layers, imageToPull, authToken)

	//copyDockerExplorer(command, file)
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: file,
		Cloneflags: syscall.CLONE_NEWPID}

	error := cmd.Run()

	if error != nil {
		fmt.Printf("Error in waiting on process: %v\n", error)
		if _, ok := error.(*exec.ExitError); ok {
			os.Exit(cmd.ProcessState.ExitCode())
		}
		fmt.Printf("Err: %v", error)
		os.Exit(1)
	}

	os.Exit(cmd.ProcessState.ExitCode())
}

func makeDirectory(dockerExplorerPath string) {
	args := []string{"-p", dockerExplorerPath}
	cmd := exec.Command("mkdir", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	error := cmd.Run()
	if error != nil {
		fmt.Printf("Error while making directory: %v\n", error)
		if _, ok := error.(*exec.ExitError); ok {
			os.Exit(cmd.ProcessState.ExitCode())
		}
		fmt.Printf("Err: %v", error)
		os.Exit(1)
	}
}

func pullLayers(baseDirectory string, fsLayers []layer, imageToPull string, authToken string) {
	client := &http.Client{}
	makeDirectory(baseDirectory)
	for _, layer := range fsLayers {
		url := fmt.Sprintf("https://registry.hub.docker.com/v2/%s/blobs/%s", imageToPull, layer.BlobSum)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
		response, _ := client.Do(req)
		defer response.Body.Close()
		buf := bytes.NewBuffer(make([]byte, 0, response.ContentLength))
		_, _ = buf.ReadFrom(response.Body)
		body := buf.Bytes()

		outputFileName := fmt.Sprintf("%s/%s", baseDirectory, layer.BlobSum)
		err := ioutil.WriteFile(outputFileName, body, 0777)
		check(err)
		tarExtract(outputFileName, baseDirectory)
		removeDanglingFile(outputFileName)
	}
}

func tarExtract(outputFileName string, baseDirectory string) {
	args := []string{"-xf", outputFileName, "-C", baseDirectory}
	cmd := exec.Command("tar", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	error := cmd.Run()
	if error != nil {
		fmt.Printf("Error while extracting directories: %v\n", error)
		if _, ok := error.(*exec.ExitError); ok {
			os.Exit(cmd.ProcessState.ExitCode())
		}
		fmt.Printf("Err: %v", error)
		os.Exit(1)
	}
}

func removeDanglingFile(fileName string) {
	args := []string{fileName}
	cmd := exec.Command("rm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	error := cmd.Run()
	if error != nil {
		fmt.Printf("Error while extracting directories: %v\n", error)
		if _, ok := error.(*exec.ExitError); ok {
			os.Exit(cmd.ProcessState.ExitCode())
		}
		fmt.Printf("Err: %v", error)
		os.Exit(1)
	}
}

func check(e error) {
	if e != nil {
		fmt.Printf("Error while copying docker exp: %v\n", e)
		panic(e)
	}
}

// func copyDockerExplorer(dockerExplorerPath string, baseDirectory string) {
// 	pathCreated := path.Join(baseDirectory, dockerExplorerPath)
// 	directoryToCreate := path.Dir(pathCreated)
// 	makeDirectory(directoryToCreate)
// 	args := []string{"-r", dockerExplorerPath, pathCreated}
// 	cmd := exec.Command("cp", args...)
// 	cmd.Stdout = os.Stdout
// 	cmd.Stderr = os.Stderr
// 	err := cmd.Run()
// 	if err != nil {
// 		fmt.Printf("Error while copying docker exp: %v\n", err)
// 		if _, ok := err.(*exec.ExitError); ok {
// 			os.Exit(cmd.ProcessState.ExitCode())
// 		}
// 		os.Exit(1)
// 	}
// }

type authToken struct {
	Token string
}

type layer struct {
	BlobSum string
}

func getAuthToken(imageToPull string) string {
	response, err := http.Get(fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull,push", imageToPull))
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	var data authToken
	json.Unmarshal(contents, &data)
	return data.Token

}

func getManifest(imageToPull string, authToken string) []layer {
	client := &http.Client{}
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/%s/manifests/latest", imageToPull)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	response, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("%s", err)
		os.Exit(1)
	}

	var objmap map[string]*json.RawMessage
	json.Unmarshal(contents, &objmap)

	keys := make([]layer, 0)
	json.Unmarshal(*objmap["fsLayers"], &keys)

	return keys
}
