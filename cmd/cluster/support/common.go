package support

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

func confirmSend() error {

	fmt.Print("Continue? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return err
	}
	response := strings.ToUpper(string(responseBytes))

	if response != "Y" && response != "YES" {
		if response != "N" && response != "NO" && response != "" {
			log.Fatal("Invalid response, expected 'YES' or 'Y' (case-insensitive). ")
		}
		log.Fatalf("Exiting...")
	}
	return nil
}

func sendRequest(request *sdk.Request) (*sdk.Response, error) {

	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}
