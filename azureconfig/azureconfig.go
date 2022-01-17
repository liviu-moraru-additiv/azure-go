package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type KVInfo struct {
	Id   string
	Name string
}

type Elem struct {
	Key   string
	Label string
	Value string
}

const resourceBaseName = "hostappconfig"

const keyVaultResourceName = "appconfigkv"

const replacementForSecret = "mysecret"

var secrets = make(map[string]map[string]string)

func main() {

	env := flag.String("env", "", "The environment. ( ex. ci, ctp etc.")
	appkey := flag.String("appkey", "", "The AppKey ( ex. clientservices, reportingservices etc.")

	fileName := flag.String("file", "", "Import file")
	command := flag.String("command", "", "Command (d-delete, a-setAppsettingsKey, i-importSettings, e-exportSettings)")

	flag.Parse()

	if *env == "" {
		log.Fatalf("Provide the environment. ( ex. ci, ctp etc.")
	}

	if *appkey == "" {
		log.Fatalf("Provide the AppKey ( ex. clientservices, reportingservices etc.")
	}

	if *command == "" || (*command != "d" && *command != "e" && *command != "i") {
		log.Fatalf("Command must be -d (delete) or -e (setAppsettingsKey)")
	}

	switch *command {
	case "d":
		deleteFromLabel(*env, *appkey)
	case "a":
		setAppsettingsKey(*env, *appkey)
	case "i":
		importSettings(*env, *appkey, *fileName)
	case "e":
		exportSettings(*env, *appkey, *fileName)
	}

}

func exportSettings(env string, host string, fileName string) {
	azureResourceName := resourceBaseName + "-" + env

	//Export to file
	fmt.Println("Export to file")
	cmdLine := "az appconfig kv export -n " + azureResourceName + " --label " + host + " -d file --path " + fileName + " --format json --yes --separator :"
	cmd := getCommand(cmdLine)
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Error running export to file: %v", err)
	}

	// Read file
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatalf("Cannot retrieve file content after export")
	}
	scontent := string(content)

	// Replace secrets
	fmt.Println("Replace secrets")

	regStr := `\{\s*\"uri\"\:\s*\"\S*\"\s*\}`
	r := regexp.MustCompile(regStr)
	scontent = r.ReplaceAllString(scontent, `"`+replacementForSecret+`"`)

	//Write to file
	fmt.Println("Write to file")
	os.WriteFile(fileName, []byte(scontent), 0644)

}

func importSecretKeys() {
	var kv []KVInfo
	cmdLine := "az keyvault secret list --vault-name " + keyVaultResourceName
	cmd := getCommand(cmdLine)
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("Cannot retrieve key vault keys")
	}
	json.Unmarshal(out, &kv)
	for _, k := range kv {
		sv := strings.Split(k.Name, "-")
		if len(sv) >= 3 {
			appKey := sv[0] + "-" + sv[1]
			kvId := strings.Join(sv[2:], ":")
			if v, ok := secrets[appKey]; ok {
				v[kvId] = k.Id
			} else {
				newMap := make(map[string]string)
				newMap[kvId] = k.Id
				secrets[appKey] = newMap

			}
		}
	}

}
func importSettings(env string, host string, fileName string) {

	importSecretKeys()
	azureResourceName := resourceBaseName + "-" + env

	if fileName == "" {
		log.Fatalf("Provide the name of the file to be imported.")
	}
	_, err := os.Stat(fileName)
	if err != nil {
		log.Fatalf("Cannot open file %s", fileName)
	}

	//Cleanup current label
	deleteFromLabel(env, host)

	//Import from json file into current label

	importedMap := importLabel(azureResourceName, host, fileName)
	if s, ok := secrets[env+"-"+host]; ok {
		for k, v := range s {
			if _, ok := importedMap[k]; ok {
				cmdLine := "az appconfig kv set-keyvault --yes -n " + azureResourceName + " --key " + k + " --label " + host + " --secret-identifier " + v
				cmd := getCommand(cmdLine)
				err = cmd.Run()
				if err != nil {
					log.Fatalf("Cannot set the secret %s for key %s.", v, k)
				}
			}
		}
	}

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

func deleteFromLabel(env string, host string) {
	azureResourceName := resourceBaseName + "-" + env
	cmdLine := "az appconfig kv delete --name " + azureResourceName + " --yes --key * --label "
	if host == "" {
		cmdLine += "\\0"
	} else {
		cmdLine += host
	}
	cmd := getCommand(cmdLine)

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}
}

func setAppsettingsKey(env string, host string) {
	azureResourceName := resourceBaseName + "-" + env
	tempFile := "temp.json"
	cmdLine := "az appconfig kv export --name " + azureResourceName + "    --destination file --path " + tempFile + " --format json --separator : --yes"
	if host != "" {
		cmdLine += " --label " + host
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

	cmdLine = "az appconfig kv set --name " + azureResourceName + " --key appsettings --label " + host + " --content-type application/json --yes --value " + text
	cmd = getCommand(cmdLine)
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Cannot set the key appsettings. Error: %s", err)
	}

}

func copyToTempLabel(azureResourceName string, host string, tmpLabel string) {

	cmdLine := "az appconfig kv export --yes --name " + azureResourceName + " -d appconfig --key *  --label " + host + " --dest-name " + azureResourceName + " --dest-label " + tmpLabel
	cmd := getCommand(cmdLine)
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Error running command %s. Error: %s", cmdLine, err)
	}

}

func getCommand(cmd string) *exec.Cmd {
	fmt.Printf("Command line: %s\n", cmd)
	space := regexp.MustCompile(`\s+`)
	cmd = space.ReplaceAllString(cmd, " ")
	args := strings.Split(cmd, " ")

	command := exec.Command(args[0], args[1:]...)
	command.Stderr = os.Stderr
	return command
}
