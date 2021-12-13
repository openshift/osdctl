package env

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	ocmconfig "github.com/openshift-online/ocm-cli/pkg/config"
	"github.com/openshift/osdctl/pkg/config"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type Options struct {
	DeleteEnv        bool
	TempEnv          bool
	ResetEnv         bool
	ExportKubeConfig bool

	Alias string

	// Options for OCM login
	ClusterId   string
	LoginScript string

	// Options for individual cluster login
	Username   string
	Password   string
	Url        string
	Kubeconfig string
}

type OcEnv struct {
	Path    string
	Exists  bool
	Options *Options
	Config  config.Config
}

func NewCmdEnv(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	options := Options{}
	config := config.Load()

	env := OcEnv{
		Options: &options,
		Config:  config,
	}
	envCmd := &cobra.Command{
		Use:               "env [flags] [env-alias]",
		Short:             "Create an environment to interact with a cluster",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		Run:               env.RunCommand,
	}
	envCmd.Flags().BoolVarP(&options.DeleteEnv, "delete", "d", false, "Delete environment")
	envCmd.Flags().BoolVarP(&options.TempEnv, "temp", "t", false, "Delete environment on exit")
	envCmd.Flags().BoolVarP(&options.ResetEnv, "reset", "r", false, "Reset environment")
	envCmd.Flags().BoolVarP(&options.ExportKubeConfig, "export-kubeconfig", "k", false, "Output export kubeconfig statement, to use environment outside of the env directory")

	envCmd.Flags().StringVarP(&options.ClusterId, "cluster-id", "c", "", "Cluster ID")
	envCmd.Flags().StringVarP(&options.LoginScript, "login-script", "l", "", "OCM login script to execute in a loop in ocb every 30 seconds")

	envCmd.Flags().StringVarP(&options.LoginScript, "username", "u", "", "Username for individual cluster login")
	envCmd.Flags().StringVarP(&options.LoginScript, "password", "p", "", "Password for individual cluster login")
	envCmd.Flags().StringVarP(&options.LoginScript, "api", "a", "", "OpenShift API URL for individual cluster login")
	envCmd.Flags().StringVarP(&options.LoginScript, "kubeconfig", "K", "", "KUBECONFIG file to use in this env (will be copied to the environment dir)")

	return envCmd
}

func (e *OcEnv) RunCommand(cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		e.Options.Alias = args[0]
	}
	if e.Options.ClusterId == "" && e.Options.Alias == "" {
		cmd.Help()
		log.Fatal("ClusterId or Alias required")
	}

	if e.Options.Alias == "" {
		log.Println("No Alias set, using cluster ID")
		e.Options.Alias = e.Options.ClusterId
	}

	e.Path = os.Getenv("HOME") + "/ocenv/" + e.Options.Alias
	e.Setup()

	if e.Options.DeleteEnv {
		e.Delete()
		return
	}
	if e.Options.ExportKubeConfig {
		e.PrintKubeConfigExport()
		return
	}
	e.Migration()
	e.Start()
	if e.Options.TempEnv {
		e.Delete()
	}
}

func (e *OcEnv) Setup() {
	if e.Options.ResetEnv {
		e.Delete()
	}
	e.ensureEnvDir()
	if !e.Exists || e.Options.ResetEnv {
		fmt.Println("Setting up environment...")
		e.createBins()
		e.ensureEnvVariables()
		e.createKubeconfig()
	}
}

func (e *OcEnv) PrintKubeConfigExport() {
	fmt.Printf("export KUBECONFIG=%s\n", e.Path+"/kubeconfig.json")
}

func (e *OcEnv) Migration() {
	if _, err := os.Stat(e.Path + "/.envrc"); err == nil {
		fmt.Println("Migrating from .envrc to .ocenv...")

		file, err := os.Open(e.Path + "/.envrc")
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "export CLUSTERID=") {
				e.Options.ClusterId = strings.ReplaceAll(line, "export CLUSTERID=", "")
				e.Options.ClusterId = strings.ReplaceAll(line, "\"", "")
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}

		e.ensureEnvVariables()

		os.Remove(e.Path + "/.envrc")
	}
}
func (e *OcEnv) Start() {
	shell := os.Getenv("SHELL")

	fmt.Print("Switching to OpenShift environment " + e.Options.Alias + "\n")
	fmt.Printf("%s %s\n", shell, e.Path+"/.ocenv")
	cmd := exec.Command(shell, "--rcfile", e.Path+"/.ocenv")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = e.Path
	_ = cmd.Run() // add error checking

	e.killChilds()

	fmt.Printf("Exited OpenShift environment\n")

}
func (e *OcEnv) killChilds() {
	file, err := os.Open(e.Path + "/.killpids")

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Nothing to kill")
			return
		}
		log.Fatalf("Failed to read file .killpids: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	scanner.Split(bufio.ScanLines)
	var text []string

	for scanner.Scan() {
		text = append(text, scanner.Text())
	}

	for _, pid := range text {
		fmt.Printf("Stopping process %s\n", pid)
		pidNum, err := strconv.Atoi(pid)
		if err != nil {
			log.Printf("failed to read PID %s, you may need to clean up manually: %v\n", pid, err)
		}
		err = syscall.Kill(pidNum, syscall.SIGTERM)
		if err != nil {
			log.Printf("failed to stop child processes %s, you may need to clean up manually: %v\n", pid, err)
		}
	}

	err = os.Remove(e.Path + "/.killpids")
	if err != nil {
		log.Printf("failed to delete .killpids, you may need to clean it up manually: %v\n", err)
	}

}
func (e *OcEnv) Delete() {
	fmt.Printf("Cleaning up OpenShift environment %s\n", e.Options.Alias)
	os.RemoveAll(e.Path)
}

func (e *OcEnv) ensureEnvDir() {
	if _, err := os.Stat(e.Path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(e.Path, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	e.Exists = true
}

func (e *OcEnv) ensureEnvVariables() {
	envContent := `
export KUBECONFIG="` + e.Path + `/kubeconfig.json"
export OCM_CONFIG="` + e.Path + `/ocm.json"
export PATH="` + e.Path + `/bin:` + os.Getenv("PATH") + `"
`
	if e.Options.ClusterId != "" {
		envContent = envContent + "export CLUSTERID=" + e.Options.ClusterId + "\n"
	}
	direnvfile, err := os.Create(e.Path + "/.ocenv")
	if err != nil {
		log.Fatal(err)
	}
	_, err = direnvfile.WriteString(envContent)
	if err != nil {
		log.Fatal(err)
	}
	defer direnvfile.Close()
}

func (e *OcEnv) createBins() {
	if _, err := os.Stat(e.binPath()); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(e.binPath(), os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}
	e.createBin("oct", "ocm tunnel "+e.Options.ClusterId)
	if e.Options.Kubeconfig == "" {
		e.createBin("ocl", e.generateLoginCommand())
	}
	e.createBin("ocd", "ocm describe cluster "+e.Options.ClusterId)
	loginScript := e.getLoginScript()
	ocb := `
#!/bin/bash

set -euo pipefail

sudo ls`
	if loginScript != "" {
		ocb += `
while true; do
  sleep 30s
  ` + loginScript + `
done &
` + loginScript + `
echo $! >> .killpids
`
		ocb += `
ocm-backplane tunnel ` + e.Options.ClusterId + ` &
echo $! >> .killpids
sleep 5s
ocm backplane login ` + e.Options.ClusterId + `
`
	}
	e.createBin("ocb", ocb)
}

func (e *OcEnv) generateLoginCommand() string {
	if e.Options.Username != "" {
		return e.generateLoginCommandIndividualCluster()
	}
	return "ocm cluster login --token " + e.Options.ClusterId
}

func (e *OcEnv) generateLoginCommandIndividualCluster() string {
	if e.Options.Url == "" {
		panic("Username set but no API Url. Use --api to specify it.")
	}
	cmd := "oc login -u " + e.Options.Username
	if e.Options.Password != "" {
		cmd += " -p " + e.Options.Password
	}
	cmd += " " + e.Options.Url
	return cmd
}

func (e *OcEnv) getLoginScript() string {
	if e.Options.LoginScript != "" {
		fmt.Printf("Using login script from -l argument: %s\n", e.Options.LoginScript)
		return e.Options.LoginScript
	}
	cfg, err := ocmconfig.Load()
	if err != nil || cfg == nil {
		fmt.Println("Can't read ocm config. Ignoring.")
		return ""
	}
	if val, ok := e.Config.LoginScripts[cfg.URL]; ok {
		fmt.Printf("Using login script from config: %s\n", val)
		return val
	}
	return ""
}

func (e *OcEnv) createBin(cmd, content string) {
	filepath := e.binPath() + "/" + cmd
	scriptfile := e.ensureFile(filepath)
	defer scriptfile.Close()
	_, err := scriptfile.WriteString(content)
	if err != nil {
		panic(fmt.Errorf("error writing to file %s: %v", filepath, err))
	}
	err = os.Chmod(filepath, 0744)
	if err != nil {
		log.Fatalf("Can't update permissions on file %s: %v", filepath, err)
	}
}

func (e *OcEnv) createKubeconfig() {
	if e.Options.Kubeconfig != "" {
		input, err := ioutil.ReadFile(e.Options.Kubeconfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		err = ioutil.WriteFile(e.Path+"/kubeconfig.json", input, 0644)
		if err != nil {
			panic(fmt.Errorf("error creating %s: %v", e.Path+"/kubeconfig.json", err))
		}
	}
}

func (e *OcEnv) ensureFile(filename string) (file *os.File) {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		file, err = os.Create(filename)
		if err != nil {
			log.Fatalf("Can't create file %s: %v", filename, err)
		}
	}
	return
}

func (e *OcEnv) binPath() string {
	return e.Path + "/bin"
}
