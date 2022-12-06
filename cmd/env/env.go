package env

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	ocmconfig "github.com/openshift-online/ocm-cli/pkg/config"
	config "github.com/openshift/osdctl/pkg/envConfig"
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

var Config_Filepath = "/.osdctl.yaml"

func NewCmdEnv(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	options := Options{}
	config := config.LoadYaml(Config_Filepath)

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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			validEnvs := []string{}
			files, err := os.ReadDir(os.Getenv("HOME") + "/ocenv/")
			if err != nil {
				return validEnvs, cobra.ShellCompDirectiveNoFileComp
			}
			for _, f := range files {
				if f.IsDir() && strings.HasPrefix(f.Name(), toComplete) {
					validEnvs = append(validEnvs, f.Name())
				}
			}

			return validEnvs, cobra.ShellCompDirectiveNoFileComp
		},
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
		err := cmd.Help()
		if err != nil {
			fmt.Println("could not print help")
			return
		}
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

func (e *OcEnv) Start() {
	shell := os.Getenv("SHELL")

	fmt.Print("Switching to OpenShift environment " + e.Options.Alias + "\n")
	// ignore the following line in linter, only way to fix this is via setting a
	// constant string for the exec.Command
	cmd := exec.Command(shell) //#nosec G204 -- shell cannot be constant

	path := filepath.Clean(e.Path + "/.ocenv")
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Println("Error closing file: ", path)
			return
		}
	}()
	scanner := bufio.NewScanner(file)
	cmd.Env = os.Environ()
	for scanner.Scan() {
		line := scanner.Text()
		cmd.Env = append(cmd.Env, line)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = e.Path
	_ = cmd.Run() // add error checking

	e.killChildren()

	fmt.Printf("Exited OpenShift environment\n")

}

func (e *OcEnv) killChildren() {
	path := filepath.Join(e.Path, "/.killpds")
	file, err := os.Open(path) //#nosec G304 -- Potential file inclusion via variable

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Nothing to kill")
			return
		}
		log.Fatalf("Failed to read file .killpids: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("Error while closing file: ", path)
			return
		}
	}(file)

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

	err = os.Remove(path)
	if err != nil {
		log.Printf("failed to delete .killpids, you may need to clean it up manually: %v\n", err)
		return
	}

}
func (e *OcEnv) Delete() {
	fmt.Printf("Cleaning up OpenShift environment %s\n", e.Options.Alias)
	err := os.RemoveAll(e.Path)
	if err != nil {
		fmt.Println("Error while calling os.RemoveAll", err.Error())
		return
	}
	return
}

func (e *OcEnv) ensureEnvDir() {
	if _, err := os.Stat(e.Path); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(e.Path, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	e.Exists = true
}

func (e *OcEnv) ensureEnvVariables() {
	envContent := `
KUBECONFIG=` + e.Path + `/kubeconfig.json
OCM_CONFIG=` + e.Path + `/ocm.json
PATH=` + e.Path + `/bin:` + os.Getenv("PATH") + `
`
	if e.Options.ClusterId != "" {
		envContent = envContent + "CLUSTERID=" + e.Options.ClusterId + "\n"
	}
	direnvfile := e.ensureFile(e.Path + "/.ocenv")
	_, err := direnvfile.WriteString(envContent)
	if err != nil {
		log.Fatal(err)
	}
	defer func(direnvfile *os.File) {
		err := direnvfile.Close()
		if err != nil {
			fmt.Println("Error while calling direnvFile.Close(): ", err.Error())
			return
		}
	}(direnvfile)

	zshenvfile := e.ensureFile(e.Path + "/.zshenv")
	_, err = zshenvfile.WriteString("source .ocenv")
	if err != nil {
		log.Fatal(err)
	}
	defer func(direnvfile *os.File) {
		err := direnvfile.Close()
		if err != nil {
			fmt.Println("Error while calling direnvFile.Close(): ", err.Error())
			return
		}
	}(direnvfile)
	return
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
	}
	ocb += `
ocm-backplane tunnel ` + e.Options.ClusterId + ` &
echo $! >> .killpids
sleep 5s
ocm backplane login ` + e.Options.ClusterId + `
`
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
	path := filepath.Join(e.binPath(), cmd)
	scriptfile := e.ensureFile(path)
	defer func(scriptfile *os.File) {
		err := scriptfile.Close()
		if err != nil {
			fmt.Println("Error closing file: ", path)
			return
		}
	}(scriptfile)
	_, err := scriptfile.WriteString(content)
	if err != nil {
		panic(fmt.Errorf("error writing to file %s: %v", path, err))
	}
	err = os.Chmod(path, 0700) //#nosec G302 -- Expect file permissions to be 0600 or less, not applicable here, because it's an executable
	if err != nil {
		log.Fatalf("Can't update permissions on file %s: %v", path, err)
		return
	}
}

func (e *OcEnv) createKubeconfig() {
	if e.Options.Kubeconfig != "" {
		input, err := os.ReadFile(e.Options.Kubeconfig)
		if err != nil {
			fmt.Println(err)
			return
		}

		path := filepath.Join(e.Path, "/kubeconfig.json")
		err = os.WriteFile(path, input, 0600)
		if err != nil {
			panic(fmt.Errorf("error creating %s: %v", path, err))
		}
	}
}

func (e *OcEnv) ensureFile(filename string) (file *os.File) {
	filename = filepath.Clean(filename)
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		file, err = os.Create(filename) //#nosec G304 -- Potential file inclusion via variable
		if err != nil {
			log.Fatalf("Can't create file %s: %v", filename, err)
		}
	}
	return
}

func (e *OcEnv) binPath() string {
	return e.Path + "/bin"
}
