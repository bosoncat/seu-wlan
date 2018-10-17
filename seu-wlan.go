package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var SEU_WLAN_LOGIN_URL = "http://w.seu.edu.cn/index.php/index/login"

// Loggers
var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

// Command line options
type Options struct {
	config   string
	username string
	password string
	interval int
}

var options *Options

// Runtime error
type runtimeError struct {
	errType string
	errHint string
}

func (err *runtimeError) Error() string {
	return fmt.Sprintf("[%v]  %v", err.errType, err.errHint)
}

func loggerInit() {
	Info = log.New(os.Stdout, "[Info]    ", log.Ldate|log.Ltime)
	Warning = log.New(os.Stdout, "[Warning] ", log.Ldate|log.Ltime)
	Error = log.New(os.Stdout, "[Error]   ", log.Ldate|log.Ltime)
}

func init() {
	// init command options parser
	options = &Options{}
	flag.StringVar(&options.username, "u", "", "Your card number. (Required)")
	flag.StringVar(&options.password, "p", "", "Your password. (Required)")
	flag.StringVar(&options.config, "c", "", "Your config file.")
	flag.IntVar(&options.interval, "i", 0, "Enable this plugin run in loop and request seu-wlan login server.")
	flag.Usage = func() {
		fmt.Println("Usage: seu-wlan [options] param")
		flag.PrintDefaults()
	}

	// init loggers
	loggerInit()
}

func main() {
	flag.Parse()

	err := checkOptions(options)
	if err != nil {
		Error.Println(err)
		flag.Usage()
		os.Exit(1)
	}

	param := encodeParam(options)

	if options.interval > 0 {
		runInLoop(param, options.interval)
	} else {
		runOnce(param)
	}
	return
}

func encodeParam(options *Options) url.Values {
	b64pass := base64.StdEncoding.EncodeToString([]byte(options.password))
	return url.Values{"username": {options.username},
		"password":      {string(b64pass)},
		"enablemacauth": {"0"} }
}

func loginRequest(param url.Values, interval int) (error, map[string]interface{}) {
	var client *http.Client
	if interval != 0 {
		client = &http.Client{Timeout: time.Second * time.Duration(interval)}
	} else {
		client = &http.Client{}
	}
	response, err := client.PostForm(SEU_WLAN_LOGIN_URL, param)
	if err != nil {
		return &runtimeError{"HTTP Request Error", "error occurred when sending post request"}, nil
	}
	defer response.Body.Close()

	loginMsgRaw, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return &runtimeError{"Read Response Error", "error occurred when reading response from server"}, nil
	}

	var loginMsgJson map[string]interface{}
	err = json.Unmarshal(loginMsgRaw, &loginMsgJson)
	if err != nil {
		return &runtimeError{"Parse JSON Error", "error occurred when parsing JSON format response"}, nil
	}
	return nil, loginMsgJson
}

func emitLog(err error, loginMsgJson map[string]interface{}) {
	if err != nil {
		Error.Println(err)
	} else if loginMsgJson["status"] == 1.0 {
		Info.Printf("%v\tlogin user: %v\tlogin ip: %v\tlogin loc: %v\n",
			loginMsgJson["info"],
			loginMsgJson["logout_username"],
			loginMsgJson["logout_ip"],
			loginMsgJson["logout_location"])
	} else {
		Info.Println(loginMsgJson["info"])
	}
}

func runInLoop(param url.Values, interval int) {
	for {
		err, loginMsgJson := loginRequest(param, interval)
		emitLog(err, loginMsgJson)
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func runOnce(param url.Values) {
	err, loginMsgJson := loginRequest(param, 0)
	emitLog(err, loginMsgJson)
}

func checkOptions(options *Options) error {
	if options.config != "" {
		/* read from config file */
		err := readConfigFile(options.config, options)
		if err != nil {
			return err
		}
	}
	if options.username == "" || options.password == "" {
		return &runtimeError{"Command Parse Error", "username and password are required."}
	} else if options.interval < 0 {
		return &runtimeError{"Command Parse Error", "-i option cannot be less than 0."}
	}
	return nil
}

func readConfigFile(path string, options *Options) error {
	jsonFile, err := os.Open(path)
	defer jsonFile.Close()

	if err != nil {
		return &runtimeError{"Config File Parse Error", "an error occurred when reading config file"}
	}
	byteVal, err := ioutil.ReadAll(jsonFile)

	if err != nil {
		return &runtimeError{"Config File Parse Error", "an error occurred when reading config file"}
	}

	var configJson map[string]interface{}
	err = json.Unmarshal(byteVal, &configJson)

	if err != nil {
		return &runtimeError{"Config File Parse Error", "an error occurred when parsing config file"}
	}

	if configJson["username"] == nil || configJson["password"] == nil {
		return &runtimeError{"Config File Parse Error", "username and password are required"}
	}

	switch ty := configJson["username"].(type) {
	default:
		return &runtimeError{"Config File Parse Error", fmt.Sprintf("username should be string format, not %T", ty)}
	case string:
		options.username = configJson["username"].(string)
	}

	switch ty := configJson["password"].(type) {
	default:
		return &runtimeError{"Config File Parse Error", fmt.Sprintf("password should be string format, not %T", ty)}
	case string:
		options.password = configJson["password"].(string)
	}

	if configJson["interval"] != nil {
		switch ty := configJson["interval"].(type) {
		default:
			return &runtimeError{"Config File Parse Error", fmt.Sprintf("interval should be integer, not %T", ty)}
		case float64:
			options.interval = int(configJson["interval"].(float64))
		}
	}

	return nil
}
