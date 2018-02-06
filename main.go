package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"github.com/yosssi/gmq/mqtt"
	"github.com/yosssi/gmq/mqtt/client"
)

type displayCommands struct {
	OnCommand  []string
	OffCommand []string
}

var commands map[string]displayCommands

var osType = runtime.GOOS

func main() {

	commands = map[string]displayCommands{
		"darwin": displayCommands{
			OnCommand:  []string{"caffeinate", "-u", "-t", "2"},
			OffCommand: []string{"pmset", "displaysleepnow"},
		},
		"linux": displayCommands{
			OnCommand:  []string{"xset", "dpms", "force", "on"},
			OffCommand: []string{"xset", "dpms", "force", "off"},
		},
	}

	if _, ok := commands[osType]; !ok {
		log.Fatalf("Unsupported OS %s - exiting.", osType)
	}

	log.Printf("Using commands for %s: %#v", osType, commands[osType])

	hostname, herr := os.Hostname()
	if herr != nil {
		log.Panicf("Couldn't determine my hostname: %v", herr)
	}
	clientID := fmt.Sprintf("xset-listener-%s-%d", hostname, os.Getpid())
	log.Printf("Starting up mqtt listener with clientid [%s]", clientID)

	// Set up channel on which to send signal notifications.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill)

	// Create an MQTT Client.
	cli := client.New(&client.Options{
		// Define the processing of the error handler.
		ErrorHandler: func(err error) {
			log.Panicf("Encountered MQTT client error: %v", err)
		},
	})

	// Terminate the Client.
	defer cli.Terminate()

	// Connect to the MQTT Server.
	// TODO: allow overriding of these options
	err := cli.Connect(&client.ConnectOptions{
		Network:   "tcp",
		Address:   "mqtt:1883",
		ClientID:  []byte(clientID),
		KeepAlive: 30,
	})
	if err != nil {
		log.Panic(err)
	}

	// Subscribe to topics.
	err = cli.Subscribe(&client.SubscribeOptions{
		SubReqs: []*client.SubReq{
			&client.SubReq{
				TopicFilter: []byte("home/monitors/all"),
				QoS:         mqtt.QoS0,
				// Define the processing of the message handler.
				Handler: analyzePayload,
			},
			&client.SubReq{
				TopicFilter: []byte("home/monitors/myhost"),
				QoS:         mqtt.QoS0,
				Handler:     analyzePayload,
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}

	// Wait for receiving a signal.
	<-sigc

	// Disconnect the Network Connection.
	if err := cli.Disconnect(); err != nil {
		log.Panic(err)
	}
}

func analyzePayload(topic []byte, payload []byte) {
	log.Printf("got: [t] %s  [m] %s\n", topic, payload)

	var cmd *exec.Cmd

	switch t := fmt.Sprintf("%s", payload); t {
	case "on":
		cmd = exec.Command(commands[osType].OnCommand[0], commands[osType].OnCommand[1:]...)
	case "off":
		cmd = exec.Command(commands[osType].OffCommand[0], commands[osType].OffCommand[1:]...)
	default:
		log.Printf("Unknown payload [%s], ignoring", payload)
	}

	if cmd != nil {
		log.Printf("executing %s", cmd.Args)
		if err := cmd.Run(); err != nil {
			output, _ := cmd.CombinedOutput()
			log.Fatalf("Error running command: %v\n%s", err, output)
		}
	}

}
