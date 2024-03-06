package silence

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

type ID struct {
	ID string `json:"id"`
}

type Matchers struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Silence struct {
	ID       string		`json:"id"`
	Matchers []Matchers `json:"matchers"`

	//Status    Status `json:"status"`	//some issue in using Status
	Comment   string `json:"comment"`
	CreatedBy string `json:"createdBy"`
	EndsAt    string `json:"endsAt"`
	StartsAt  string `json:"startsAt"`
}

func NewCmdListSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "list <cluster-id>",
		Short:             "List all silences",
		Long:              `print the list of silences`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ListSilence(args[0])
		},
	}
}

func ListSilence(clusterID string) {
	var silences []Silence

	silenceCmd := []string{"amtool", "silence", "--alertmanager.url", LocalHostUrl, "-o", "json"}

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	op, err := ExecInPod(kubeconfig, clientset, silenceCmd)
	if err != nil {
		fmt.Println(err)
	}

	opSlice := []byte(op)
	err = json.Unmarshal(opSlice, &silences)
	if err != nil {
		fmt.Println("Error in unmarshaling the data", err)
	}

	fmt.Printf("Silence Information:\n")
	if len(silences) > 0 {
		for _, silence := range silences {
			printSilence(silence)
		}
	} else {
		fmt.Println("No silences found, all silence has been cleared.")
	}
}

func printSilence(silence Silence) {
	id, matchers, created, starts, end, comment := silence.ID, silence.Matchers, silence.CreatedBy, silence.StartsAt, silence.EndsAt, silence.Comment
	fmt.Println("-------------------------------------------")
	for _, matcher := range matchers {
		fmt.Printf("  SilenceID:		%s\n", id)
		//fmt.Printf("  Status:		%s\n", status)
		fmt.Printf("  Created By:		%s\n", created)
		fmt.Printf("  Starts At:		%s\n", starts)
		fmt.Printf("  Ends At:		%s\n", end)
		fmt.Printf("  Comment:		%s\n", comment)
		fmt.Printf("  AlertName:		%s\n", matcher.Value)
	}
	fmt.Println("---------------------------------------------")
}
