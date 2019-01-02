package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-steplib/bitrise-step-android-unit-test/cache"
	"github.com/bitrise-tools/go-android/gradle"
	"github.com/bitrise-tools/go-steputils/stepconf"
	shellquote "github.com/kballard/go-shellquote"
)

// Config ...
type Config struct {
	ProjectLocation   string `env:"project_location,dir"`
	ReportPathPattern string `env:"report_path_pattern"`
	Module            string `env:"module"`
	Arguments         string `env:"arguments"`
	CacheLevel        string `env:"cache_level,opt[none,only_deps,all]"`
	DeployDir         string `env:"BITRISE_DEPLOY_DIR,dir"`
}

func getArtifacts(gradleProject gradle.Project, started time.Time, pattern string) (artifacts []gradle.Artifact, err error) {
	artifacts, err = gradleProject.FindArtifacts(started, pattern, true)
	if err != nil {
		return
	}

	if len(artifacts) == 0 {
		if !started.IsZero() {
			log.Warnf("No artifacts found with pattern: %s that has modification time after: %s", pattern, started)
			log.Warnf("Retrying without modtime check....")
			fmt.Println()
			return getArtifacts(gradleProject, time.Time{}, pattern)
		}
		log.Warnf("No artifacts found with pattern: %s without modtime check", pattern)
		log.Warnf("If you have changed default report export path in your gradle files then you might need to change ReportPathPattern accordingly.")
	}
	return
}

func failf(f string, args ...interface{}) {
	log.Errorf(f, args...)
	os.Exit(1)
}

func runDetektTask(config Config) error {
	gradleProject, err := gradle.NewProject(config.ProjectLocation)
	if err != nil {
		return fmt.Errorf("Failed to open project, error: %s", err)
	}

	detektTask := gradleProject.GetTask("detekt")

	started := time.Now()

	args, err := shellquote.Split(config.Arguments)
	if err != nil {
		return fmt.Errorf("Failed to parse arguments, error: %s", err)
	}

	log.Infof("Run detekt:")

	emptyVariants := gradle.Variants{}
	emptyVariants[config.Module] = append(emptyVariants[config.Module], "")

	detektCommand := detektTask.GetCommand(emptyVariants, args...)
	fmt.Println()
	log.Donef("Printable command args: %s" + detektCommand.PrintableCommandArgs())
	fmt.Println()

	taskError := detektCommand.Run()
	if taskError != nil {
		log.Errorf("Detekt task failed, error: %v", taskError)
	}
	fmt.Println()

	log.Infof("Exporting artifacts:")
	fmt.Println()

	artifacts, err := getArtifacts(gradleProject, started, config.ReportPathPattern)
	if err != nil {
		return fmt.Errorf("failed to find artifacts, error: %v", err)
	}

	if len(artifacts) > 0 {
		for _, artifact := range artifacts {
			exists, err := pathutil.IsPathExists(
				filepath.Join(config.DeployDir, artifact.Name),
			)
			if err != nil {
				return fmt.Errorf("failed to check path, error: %v", err)
			}

			artifactName := filepath.Base(artifact.Path)

			if exists {
				timestamp := time.Now().Format("20060102150405")
				ext := filepath.Ext(artifact.Name)
				name := strings.TrimSuffix(filepath.Base(artifact.Name), ext)
				artifact.Name = fmt.Sprintf("%s-%s%s", name, timestamp, ext)
			}

			log.Printf("  Export [ %s => $BITRISE_DEPLOY_DIR/%s ]", artifactName, artifact.Name)

			if err := artifact.Export(config.DeployDir); err != nil {
				log.Warnf("failed to export artifacts, error: %v", err)
			}
		}
	} else {
		log.Warnf("No artifacts found with pattern: %s", config.ReportPathPattern)
		log.Warnf("If you have changed default report file paths with lintOptions/htmlOutput or lintOptions/xmlOutput")
		log.Warnf("in your gradle files then you might need to change ReportPathPattern accordingly.")
	}

	return taskError
}

func main() {
	var config Config

	if err := stepconf.Parse(&config); err != nil {
		failf("Couldn't create step config: %v\n", err)
	}

	stepconf.Print(config)
	fmt.Println()

	if err := runDetektTask(config); err != nil {
		failf("%s", err)
	}

	fmt.Println()
	log.Infof("Collecting cache:")
	if warning := cache.Collect(config.ProjectLocation, cache.Level(config.CacheLevel)); warning != nil {
		log.Warnf("%s", warning)
	}
	log.Donef("  Done")
}
