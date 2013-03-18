package goxc

/*
   Copyright 2013 Am Laher

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	//Tip for Forkers: please 'clone' from my url and then 'pull' from your url. That way you wont need to change the import path.
	//see https://groups.google.com/forum/?fromgroups=#!starred/golang-nuts/CY7o2aVNGZY
	"github.com/laher/goxc/archive"
	"github.com/laher/goxc/config"
	"path/filepath"
	"runtime"
	"strings"
)

func signBinary(binPath string, settings config.Settings) error {
	cmd := exec.Command("codesign")
	cmd.Args = append(cmd.Args, "-s", settings.Codesign, binPath)
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func runTaskXC(destPlatforms [][]string, workingDirectory string, settings config.Settings) {
	for _, platformArr := range destPlatforms {
		destOs := platformArr[0]
		destArch := platformArr[1]
		XCPlat(destOs, destArch, workingDirectory, settings)
	}
}


// XCPlat: Cross compile for a particular platform
// 'isFirst' is used simply to determine whether to start a new downloads.md page
// 0.3.0 - breaking change - changed 'call []string' to 'workingDirectory string'.
func XCPlat(goos, arch string, workingDirectory string, settings config.Settings) string {
	log.Printf("building for platform %s_%s.", goos, arch)
	relativeDir := filepath.Join(settings.GetFullVersionName(), goos+"_"+arch)

	appName := getAppName(workingDirectory)

	outDestRoot := getOutDestRoot(appName, settings)
	outDir := filepath.Join(outDestRoot, relativeDir)
	os.MkdirAll(outDir, 0755)

	cmd := exec.Command("go")
	cmd.Args = append(cmd.Args, "build")
	if settings.GetFullVersionName() != "" {
		cmd.Args = append(cmd.Args, "-ldflags", "-X main.VERSION "+settings.GetFullVersionName()+"")
	}
	cmd.Dir = workingDirectory
	//relativeBinForMarkdown := getRelativeBin(goos, arch, appName, true)
	relativeBin := getRelativeBin(goos, arch, appName, false, settings)
	cmd.Args = append(cmd.Args, "-o", filepath.Join(outDestRoot, relativeBin), workingDirectory)
	f, err := RedirectIO(cmd)
	if err != nil {
		log.Printf("Error redirecting IO: %s", err)
	}
	if f != nil {
		defer f.Close()
	}
	cgoEnabled := CgoEnabled(goos, arch)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED="+cgoEnabled, "GOARCH="+arch)
	if settings.IsVerbose() {
		log.Printf("'go' env: GOOS=%s CGO_ENABLED=%s GOARCH=%s", goos, cgoEnabled, arch)
		log.Printf("'go' args: %v", cmd.Args)
		log.Printf("'go' working directory: %s", cmd.Dir)
	}
	err = cmd.Start()
	if err != nil {
		log.Printf("Launch error: %s", err)
	} else {
		err = cmd.Wait()
		if err != nil {
			log.Printf("Compiler error: %s", err)
		} else {
			log.Printf("Artifact generated OK")
		}
	}
	return relativeBin
}

func runTaskCodesign(destPlatforms [][]string, outDestRoot string, appName string, settings config.Settings) {
	for _, platformArr := range destPlatforms {
		destOs := platformArr[0]
		destArch := platformArr[1]
		relativeBin := getRelativeBin(destOs, destArch, appName, false, settings)
		codesignPlat(destOs, destArch, outDestRoot, relativeBin, settings)
	}
}
func codesignPlat(goos, arch string, outDestRoot string, relativeBin string, settings config.Settings) {
	// settings.codesign only works on OS X for binaries generated for OS X.
	if settings.Codesign != "" && runtime.GOOS == DARWIN && goos == DARWIN {
		if err := signBinary(filepath.Join(outDestRoot, relativeBin), settings); err != nil {
			log.Printf("codesign failed: %s", err)
		} else {
			log.Printf("Signed with ID: %q", settings.Codesign)
		}
	}
}


func runTaskZip(destPlatforms [][]string, appName, workingDirectory, outDestRoot string, settings config.Settings) {
	for _, platformArr := range destPlatforms {
		destOs := platformArr[0]
		destArch := platformArr[1]
		zipPlat(destOs, destArch, appName, workingDirectory, outDestRoot, settings)
	}
}

func zipPlat(goos, arch, appName, workingDirectory, outDestRoot string, settings config.Settings) string {
	resources := parseIncludeResources(workingDirectory, settings.Resources.Include, settings)
	relativeBinForMarkdown := getRelativeBin(goos, arch, appName, true, settings)
	//0.4.0 use a new task type instead of artifact type
	if settings.IsTask(config.TASK_ARCHIVE) {
	//if settings.IsZipArtifact() {
		// Create ZIP archive.
		relativeBin := getRelativeBin(goos, arch, appName, false, settings)
		zipPath, err := archive.MoveBinaryToZIP(
			filepath.Join(outDestRoot, settings.GetFullVersionName()),
			filepath.Join(outDestRoot, relativeBin), appName, resources, settings.IsTask(config.TASK_REMOVE_BIN), settings)
		if err != nil {
			log.Printf("ZIP error: %s", err)
		} else {
			relativeBinForMarkdown = zipPath
			log.Printf("Artifact zipped OK")
		}
	}
	if settings.IsBinaryArtifact() {
		//TODO: move resources to folder & add links to downloads.md
	}
	return relativeBinForMarkdown
}

func runTaskDlPage(destPlatforms [][]string, appName, workingDirectory string, outDestRoot string, relativeBins map[string] string, settings config.Settings) {
	isFirst := true
	for _, platformArr := range destPlatforms {
		destOs := platformArr[0]
		destArch := platformArr[1]
		relativeBinForMarkdown := relativeBins[destOs+"_"+destArch]
		dlPagePlat(destOs, destArch, appName, workingDirectory, outDestRoot, isFirst, relativeBinForMarkdown, settings)
		isFirst = false
	}
}

func dlPagePlat(goos, arch string, appName, workingDirectory string, outDestRoot string, isFirst bool, relativeBinForMarkdown string, settings config.Settings) {
	// Output report
	reportFilename := filepath.Join(outDestRoot, settings.GetFullVersionName(), "downloads.md")
	var flags int
	if isFirst {
		log.Printf("Creating %s", reportFilename)
		flags = os.O_WRONLY | os.O_TRUNC | os.O_CREATE
	} else {
		flags = os.O_APPEND | os.O_WRONLY

	}
	f, err := os.OpenFile(reportFilename, flags, 0600)
	if err == nil {
		defer f.Close()
		if isFirst {
			_, err = fmt.Fprintf(f, "%s downloads (%s)\n------------\n\n", appName, settings.GetFullVersionName())
		}
		_, err = fmt.Fprintf(f, " * [%s %s](%s)\n", goos, arch, relativeBinForMarkdown)
	}
	if err != nil {
		log.Printf("Could not report to '%s': %s", reportFilename, err)
	}
}

func runTaskToolchain(destPlatforms [][]string, settings config.Settings) {
	for _, platformArr := range destPlatforms {
		destOs := platformArr[0]
		destArch := platformArr[1]
		BuildToolchain(destOs, destArch, settings)
	}
}


func RunTasks(workingDirectory string, settings config.Settings) {

		// 0.3.1 added clean, vet, test, install etc
		if settings.IsTask(config.TASK_CLEAN) {
			err := InvokeGo(workingDirectory, []string{config.TASK_CLEAN}, settings)
			if err != nil {
				log.Printf("Clean failed! %s", err)
				return
			}
		}
		if settings.IsTask(config.TASK_VET) {
			err := InvokeGo(workingDirectory, []string{config.TASK_VET}, settings)
			if err != nil {
				log.Printf("Vet failed! %s", err)
				return
			}
		}
		if settings.IsTask(config.TASK_TEST) {
			err := InvokeGo(workingDirectory, []string{config.TASK_TEST}, settings)
			if err != nil {
				log.Printf("Test failed! %s", err)
				return
			}
		}
		if settings.IsTask(config.TASK_FMT) {
			err := InvokeGo(workingDirectory, []string{config.TASK_FMT}, settings)
			if err != nil {
				log.Printf("Fmt failed! %s", err)
				return
			}
		}
		if settings.IsTask(config.TASK_INSTALL) {
			err := InvokeGo(workingDirectory, []string{config.TASK_INSTALL}, settings)
			if err != nil {
				log.Printf("Install failed! %s", err)
				return
			}
		}
		if settings.IsVerbose() {
			log.Printf("looping through each platform")
		}
		destOses := strings.Split(settings.Os, ",")
		destArchs := strings.Split(settings.Arch, ",")
		var destPlatforms [][]string
		for _, supportedPlatformArr := range PLATFORMS {
			supportedOs := supportedPlatformArr[0]
			supportedArch := supportedPlatformArr[1]
			for _, destOs := range destOses {
				if destOs == "" || supportedOs == destOs {
					for _, destArch := range destArchs {
						if destArch == "" || supportedArch == destArch {
							destPlatforms = append(destPlatforms, supportedPlatformArr)
/*
							if settings.IsBuildToolchain() {
								BuildToolchain(supportedOs, supportedArch)
							}
							if settings.IsXC() {
								XCPlat(supportedOs, supportedArch, workingDirectory, isFirst)
							}
							isFirst = false
*/
						}
					}
				}
			}
		}
		if settings.IsBuildToolchain() {
			runTaskToolchain(destPlatforms, settings)
		}
		if settings.IsXC() {
			runTaskXC(destPlatforms, workingDirectory, settings)
		}
}
