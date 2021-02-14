/*
Configuration package is used to read the configuration file
config.json which stores the server port for current implementation
    {
        "token": "your-telegram's-bot-token"
    }
*/
package config

import(
	"encoding/json"
	"io/ioutil"
	"log"
)

//Configuration stores the main config for the server
type Configuration struct {
	token string
}

var err error
var config Configuration

//ReadConfig will read the configuration json file to read the parameters
//which will be passed in the config file
func ReadConfig(fileName string) (Configuration, error) {
	configFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Print("Unable to read token form config file, switching to manual mode")
		return Configuration{}, err
	}

	err = json.Unmarshal(configFile, &config)
	if err != nil {
		log.Print("Invalid config file, expecting token from command line flag")
		return Configuration{}, err
	}
	return config, nil
}
