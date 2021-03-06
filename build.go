package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsouza/go-dockerclient"
	"github.com/progrium/go-shell"
	"github.com/spf13/cobra"
)

func init() {
	Glu.AddCommand(buildCmd)
}

var buildCmd = &cobra.Command{
	Use:   "build <os-list> [<pkgs>] [<name>]",
	Short: "Builds a Go project of Glider Labs",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			cmd.Usage()
			os.Exit(1)
		}
		defer shell.ErrExit()
		shell.Trace = true
		shell.Tee = os.Stdout

		if tryContainer(cmd, args) {
			return
		}

		var (
			info   = NewProjectInfo()
			osList = strings.Split(args[0], ",")
			pkgs   = optArg(args, 1, ".")
			name   = optArg(args, 2, info.Name)

			ldFlag string
		)
		if info.Version != "" {
			ldFlag = fmt.Sprintf("-ldflags \"-X main.Version=%s\"", info.Version)
		}

		if insideContainer() {
			os.Setenv("GOPATH", "/go")
			path := fmt.Sprintf("/go/src/%s", info.Repo)
			sh("mkdir -p", filepath.Dir(path))
			sh("cp -r /project", path)
			sh("cd", path) // for show
			os.Chdir(path)
		}

		sh("go get -d", pkgs)

		os.Setenv("CGO_ENABLED", "0")
		for i := range osList {
			os.Setenv("GOOS", strings.ToLower(osList[i]))
			path := shell.Path("build", strings.Title(osList[i]))
			sh("mkdir -p", path)
			sh("go build -a -installsuffix cgo", ldFlag, "-o", shell.Path(path, name), pkgs)
		}

		if insideContainer() {
			sh("rm -rf /project/build")
			sh("mv build /project")
			for i := range osList {
				sh(fmt.Sprintf("tar -czvf /artifacts/%s-%s.tgz -C /project/build/%s %s",
					name, strings.ToLower(osList[i]), strings.Title(osList[i]), name))
			}
			sh("tar -czf /artifacts/go-workspace.tgz -C /go .")
			sh("rm -rf /go")
		}
	},
}

func tryContainer(cmd *cobra.Command, args []string) bool {
	if insideContainer() {
		return false
	}
	if !dockerExistsByName("glu") {
		return false
	}
	fmt.Fprintln(os.Stderr, "* Using glu container")
	args = append(strings.Split(cmd.CommandPath(), " "), args...)
	var newCmd []string
	var binary string
	var err error
	if os.Getenv("CIRCLECI") == "true" {
		if binary, err = exec.LookPath("sudo"); err != nil {
			return false
		}
		os.Setenv("GLU_CONTAINER", "true")
		newCmd = []string{"sudo", "-E", "lxc-attach", "-n", dockerID("glu"), "--", "/bin/glu"}
		newCmd = append(newCmd, args[1:]...)
	} else {
		if binary, err = exec.LookPath("docker"); err != nil {
			return false
		}
		newCmd = []string{"docker", "exec", "glu"}
		newCmd = append(newCmd, args...)
	}
	syscall.Exec(binary, newCmd, os.Environ())
	return true
}

func insideContainer() bool {
	return os.Getenv("GLU_CONTAINER") == "true"
}

func dockerID(container string) string {
	client, err := docker.NewClientFromEnv()
	fatal(err)
	containers, err := client.ListContainers(docker.ListContainersOptions{})
	fatal(err)
	for _, cntr := range containers {
		for _, name := range cntr.Names {
			if name[1:] == container {
				return cntr.ID
			}
		}
	}
	return ""
}
