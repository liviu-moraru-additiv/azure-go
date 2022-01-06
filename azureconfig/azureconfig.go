package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Elem struct {
	Key   string
	Label string
}

func main() {

	var elems []Elem

	azureResourceName := flag.String("resource", "", "The Azure App Configuration Resource Name")
	label := flag.String("label", "", "Label ex: ClientServices")
	command := flag.String("command", "", "Command (d-delete, e-export")

	flag.Parse()

	if *azureResourceName == "" {
		log.Fatalf("Provide the Azure resource name: ex hostappconfig-ctp.")
	}

	if *label == "" {
		log.Fatalf("Provide label: ex ClientServices.")
	}
	if *command == "" || (*command != "d" && *command != "e") {
		log.Fatalf("Command must be -d (delete) or -e (export)")
	}

	if *command == "d" {
		deleteFromLabel(azureResourceName, elems, label)
	} else if *command == "e" {
		export(azureResourceName, label)
	}

}

func deleteFromLabel(azureResourceName *string, elems []Elem, label *string) {
	var wg sync.WaitGroup
	cmdLine := "az appconfig kv list --name " + *azureResourceName
	cmd := getCommand(cmdLine)
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("Command finished with error: %v", err)
	}
	fmt.Printf("Json data: %s\n", string(out))

	json.Unmarshal(out, &elems)

	for _, val := range elems {
		if strings.Contains(val.Label, *label) {
			wg.Add(1)
			time.Sleep(1 * time.Second)
			go func(val Elem) {
				defer wg.Done()
				cmdLine := "az appconfig kv delete --name " + *azureResourceName + " --yes --label " + *label + " --key " + val.Key
				fmt.Printf("Command line: %s", cmdLine)
				cmd := getCommand(cmdLine)
				fmt.Printf("Delete %s\n ", val.Key)
				err := cmd.Run()
				if err != nil {
					fmt.Printf("Error: %s\n", err)
				}
			}(val)

		}
	}
	wg.Wait()
}

func export(azureResourceName *string, label *string) {
	tempFile := "temp.json"
	cmdLine := "az appconfig kv export --name " + *azureResourceName + "    --destination file --path " + tempFile + " --label " + *label + " --format json --separator : --yes"
	cmd := getCommand(cmdLine)
	_, err := cmd.Output()
	if err != nil {
		log.Fatalf("Error: %s\n", err.Error())
	}
	content, err := os.ReadFile("temp.json")
	if err != nil {
		log.Fatalf("Cannot read the exported file. Error: %s", err)
	}
	os.Remove(tempFile)
	text := string(content)
	reg := regexp.MustCompile(`:\s+".+"`)

	text = reg.ReplaceAllString(text, `:"{{}}"`)
	text = strconv.Quote(text)
	text = strings.ReplaceAll(text, " ", "")

	cmdLine = "az appconfig kv set --name " + *azureResourceName + " --key appsettings --label " + *label + " --content-type application/json --yes --value " + text
	cmd = getCommand(cmdLine)
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Cannot set the key appsettings. Error: %s", err)
	}

}

func getCommand(cmd string) *exec.Cmd {
	space := regexp.MustCompile(`\s+`)
	cmd = space.ReplaceAllString(cmd, " ")
	args := strings.Split(cmd, " ")

	return exec.Command(args[0], args[1:]...)
}
