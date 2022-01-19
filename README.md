# Operations with Azure App Configuration using Azure CLI and Go language

## Building the program
In the root directory of the project:

```go
go build ./...
```

The execution program azureconfig.exe (Windows) or azureconfig (Linux) will be created in the root directory of the project.

## Using the application

The following commands will be run from the root directory of the project.
### Help parameter

```go
./azureconfig.exe --help
```

### Import an JSON file

Importing the appsettings-clientservices.json file from the directory appsettings-ci into the Azure Configuration resource (Environmant ci, ApplicationKey clientservices):

```go
./azureconfig.exe --command i --env ci --appkey clientservices --file appsettings-ci/appsettings-clientservices.json
```

The Azure configuration resource name for a specific environment is built from the hard-coded constant "resourceBaseName" and the environment name (ex. hostappconfig-ci for ci environment).

The secrets are defined into the Azure KeyVault resource whose name is hard-coded into the keyVaultResourceName constant (ex. appconfigkv).

### Export the values into a JSON file

To export the Azure settings from the environment ci, application key clientservices into the appsettings-clientservices-generated.json file:
```go
./azureconfig.exe --command e --env ci --appkey clientservices --file appsettings-ci/appsettings-clientservices-generated.json
```

The secret keys values will be set into the JSON file with the value of the replacementForSecret constant (ex. "mysecret").

### Delete the values for an application key

To delete the settings for the environment ci, application key misservices:

```go
./azureconfig.exe --command d --env ci --appkey misservices
```








