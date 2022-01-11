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
)

type Elem struct {
	Key   string
	Label string
	Value string
}

type duplicateKey struct {
	Label string `json:"label"`
	Key   string `json:"key"`
}

const inheritedKey = "inherited"
const templabel = "temp"

func main() {

	azureResourceName := flag.String("resource", "", "The Azure App Configuration Resource Name")
	label := flag.String("label", "", "Label ex: ClientServices")
	fileName := flag.String("file", "", "Import file")
	command := flag.String("command", "", "Command (d-delete, a-setAppsettingsKey, i-importSettings, e-exportSettings)")

	flag.Parse()

	if *azureResourceName == "" {
		log.Fatalf("Provide the Azure resource name: ex hostappconfig-ctp.")
	}

	/*if *label == "" {
		log.Fatalf("Provide label: ex ClientServices.")
	}*/
	if *command == "" || (*command != "d" && *command != "e" && *command != "i") {
		log.Fatalf("Command must be -d (delete) or -e (setAppsettingsKey)")
	}

	switch *command {
	case "d":
		deleteFromLabel(*azureResourceName, *label)
	case "a":
		setAppsettingsKey(*azureResourceName, *label)
	case "i":
		importSettings(*azureResourceName, *label, *fileName)
	case "e":
		exportSettings(*azureResourceName, *label, *fileName)
	}

}

func exportSettings(azureResourceName string, label string, fileName string) {

	defer deleteFromLabel(azureResourceName, templabel)

	//Clenup temp label
	deleteFromLabel(azureResourceName, templabel)
	//Copy the current label to tmpLabel
	copyToTempLabel(azureResourceName, label, templabel)

	//Retrieve the inherited key
	inhKeys, err := retrieveInheritedKey(azureResourceName, label)

	//Copy the inherited keys to temp label
	if err != nil {
		fmt.Printf("Cannot retrieve the %s key", inheritedKey)
	} else {
		copyInhKeysToTempLabel(azureResourceName, inhKeys)
	}

	//Export to file
	cmdLine := "az appconfig kv export -n " + azureResourceName + " --label " + templabel + " -d file --path " + fileName + " --format json --yes --separator :"
	fmt.Printf("Command line: %s\n", cmdLine)
	cmd := getCommand(cmdLine)
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Error running export to file: %v", err)
	}

}

func copyInhKeysToTempLabel(azureResourceName string, inhKeys []*duplicateKey) {
	for _, inh := range inhKeys {
		l := inh.Label
		if l == "root" {
			l = "\\0"
		}

		cmdLine := "az appconfig kv export --yes --name " + azureResourceName + " -d appconfig --key " + inh.Key + " --label " + l + " --dest-name " + azureResourceName + " --dest-label " + templabel
		fmt.Printf("Command line: %s\n", cmdLine)
		cmd := getCommand(cmdLine)
		err := cmd.Run()
		if err != nil {
			log.Fatalf("Cannot set the %s key on label %s. Error: %s", inh.Key, templabel, err)
		}

	}

}

func getKeyValue(azureResourceName string, label string, key string) (string, error) {
	cmdLine := "az appconfig kv list --name " + azureResourceName + " --fields value --key " + key + "  --label " + label
	cmd := getCommand(cmdLine)
	out, err := cmd.Output()

	if err != nil {
		return "", fmt.Errorf("error running command %s", cmdLine)
	}

	vout := make([]map[string]interface{}, 0)
	err = json.Unmarshal(out, &vout)
	if err != nil || len(vout) == 0 {
		return "", fmt.Errorf("cannot retrieve key %s from label %s", key, label)
	}

	return vout[0]["value"].(string), nil
}

func retrieveInheritedKey(azureResourceName string, label string) ([]*duplicateKey, error) {
	var dp []*duplicateKey
	out, err := getKeyValue(azureResourceName, label, inheritedKey)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve %s key", inheritedKey)
	}

	err = json.Unmarshal([]byte(out), &dp)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve %s key", inheritedKey)
	}

	return dp, nil
}

func copyToTempLabel(azureResourceName string, label string, tmpLabel string) {
	//To do exclude key appsettings and inherited
	cmdLine := "az appconfig kv export --yes --name " + azureResourceName + " -d appconfig --key *  --label " + label + " --dest-name " + azureResourceName + " --dest-label " + tmpLabel
	fmt.Printf("Command: %s\n", cmdLine)
	cmd := getCommand(cmdLine)
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Error running command %s. Error: %s", cmdLine, err)
	}

	cmdLine = "az appconfig kv delete --yes --name " + azureResourceName + " --key appsettings  --label " + tmpLabel
	fmt.Printf("Command: %s\n", cmdLine)
	cmd = getCommand(cmdLine)
	cmd.Run()

	cmdLine = "az appconfig kv delete --yes --name " + azureResourceName + " --key inherited  --label " + tmpLabel
	fmt.Printf("Command: %s\n", cmdLine)
	cmd = getCommand(cmdLine)
	cmd.Run()
}

func importSettings(azureResourceName string, label string, fileName string) {

	//Cleanup current label
	deleteFromLabel(azureResourceName, label)

	var rootMap map[string]string
	var secondLevelMap map[string]string
	var currentMap map[string]string

	//Import from json file into current label
	currentMap = importLabel(azureResourceName, label, fileName)

	//Retrieve the keys from root label
	rootMap = listKeysValues(azureResourceName, "")

	//If current label is a host label, retrieve the keys for environment label
	splitLabel := strings.Split(label, "-")
	var envLabel string
	if len(splitLabel) == 2 {
		envLabel = splitLabel[0]
		secondLevelMap = listKeysValues(azureResourceName, envLabel)
	}

	fmt.Printf("Root level no of keys: %d\n", len(rootMap))
	fmt.Printf("Seconf level no of keys: %d\n", len(secondLevelMap))
	fmt.Printf("Current level no of keys:%d\n", len(currentMap))

	//Set appsettings key with the structure of the source json file
	setAppsettingsKey(azureResourceName, label)

	//Take the duplicate keys from the upper levels and delete them from current label
	dupk := processDuplicateKeys(azureResourceName, label, currentMap, rootMap, secondLevelMap, envLabel)

	// Set the inherited key with the list of inherited keys and the source of them
	setInheritedKey(azureResourceName, label, dupk)
}

func setInheritedKey(azureResourceName string, label string, dupk []*duplicateKey) {
	b, err := json.Marshal(dupk)

	if err != nil {
		log.Fatalf("Error saving key %s:  %+v", inheritedKey, err)

	}
	cmdLine := "az appconfig kv set --name " + azureResourceName + " --key " + inheritedKey + " --label " + label + " --content-type application/json --yes --value " + string(b)
	cmd := getCommand(cmdLine)
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Cannot set the %s key. Error: %s", inheritedKey, err)
	}
}

func processDuplicateKeys(azureResourceName string, label string, currentMap map[string]string, rootMap map[string]string, secondLevelMap map[string]string, envLabel string) []*duplicateKey {
	var dupk []*duplicateKey
	for key, value := range currentMap {
		fv, fok := rootMap[key]
		sv, sok := secondLevelMap[key]
		if (fok && fv == value) || (sok && sv == value) {
			if fok {
				dupk = append(dupk, &duplicateKey{Label: "root", Key: key})
			} else {
				dupk = append(dupk, &duplicateKey{Label: envLabel, Key: key})
			}
			workerForDeleteKey(azureResourceName, label, key)

		}

	}

	return dupk
}

func importLabel(azureResourceName string, label string, fileName string) map[string]string {
	cmdLine := "az appconfig kv import --name " + azureResourceName + " --source file --path " + fileName + " --format json --separator : --yes --label "
	if label == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += label
	}
	cmd := getCommand(cmdLine)
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Command %s finished with error: %v", cmdLine, err)
	}
	return listKeysValues(azureResourceName, label)
}
func listKeysValues(azureResourceName string, label string) map[string]string {
	cmdLine := "az appconfig kv list --name " + azureResourceName + " --label "
	if label == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += label
	}
	cmd := getCommand(cmdLine)
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("Command %s finished with error: %v", cmdLine, err)
	}
	var rootElems []Elem
	json.Unmarshal(out, &rootElems)
	return transf(rootElems)
}

func transf(elems []Elem) map[string]string {
	values := make(map[string]string)
	for _, val := range elems {
		values[val.Key] = val.Value
	}
	return values
}

/*func deleteFromLabel(azureResourceName string, label string) {

	var elems []Elem
	cmdLine := "az appconfig kv list --name " + azureResourceName + " --label "
	if label == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += label
	}
	cmd := getCommand(cmdLine)
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("Command finished with error: %v", err)
	}

	json.Unmarshal(out, &elems)

	for _, elem := range elems {
		workerForDeleteKey(azureResourceName, label, elem.Key)
	}
}*/
func deleteFromLabel(azureResourceName string, label string) {
	cmdLine := "az appconfig kv delete --name " + azureResourceName + " --yes --key * --label "
	if label == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += label
	}
	fmt.Printf("Command line: %s\n", cmdLine)
	cmd := getCommand(cmdLine)

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}
}
func workerForDeleteKey(azureResourceName string, label string, key string) {

	cmdLine := "az appconfig kv delete --name " + azureResourceName + " --yes --key " + key + " --label "
	if label == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += label
	}
	fmt.Printf("Command line: %s\n", cmdLine)
	cmd := getCommand(cmdLine)

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}

}

func setAppsettingsKey(azureResourceName string, label string) {
	tempFile := "temp.json"
	cmdLine := "az appconfig kv export --name " + azureResourceName + "    --destination file --path " + tempFile + " --format json --separator : --yes"
	if label != "" {
		cmdLine += " --label " + label
	}

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

	cmdLine = "az appconfig kv set --name " + azureResourceName + " --key appsettings --label " + label + " --content-type application/json --yes --value " + text
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

	command := exec.Command(args[0], args[1:]...)
	command.Stderr = os.Stderr
	return command
}
