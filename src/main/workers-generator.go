package main

import (
	"errors"
	"flag"
	"fmt"
	dockerapi "github.com/fsouza/go-dockerclient"
	consulapi "github.com/hashicorp/consul/api"
	"log"
	"os"
	"strings"
)

var Version string

var hostIp = flag.String("ip", "", "IP for ports mapped to the host")
var internal = flag.Bool("internal", false, "Use internal ports instead of published ones")
var refreshInterval = flag.Int("ttl-refresh", 0, "Frequency with which service TTLs are refreshed")
var refreshTtl = flag.Int("ttl", 0, "TTL for services (default is no expiry)")
var forceTags = flag.String("tags", "", "Append tags for all registered services")
var resyncInterval = flag.Int("resync", 0, "Frequency with which services are resynchronized")
var deregister = flag.String("deregister", "always", "Deregister exited services \"always\" or \"on-success\"")

var worker_template string = "worker.template_ajp13.type=ajp13\n" +
	"worker.template_ajp13.connection_pool_timeout=300\n" +
	"worker.template_ajp13.connection_pool_minsize=0\n" +
	"worker.template_ajp13.ping_mode=A\n" +
	"worker.template_ajp13.ping_timeout=10000\n" +
	"worker.template_ajp13.lbfactor=10\n" +
	"worker.template_ajp13.retries=2\n" +
	"worker.template_ajp13.activation=A\n" +
	"worker.template_ajp13.recovery_options=7\n" +
	"worker.template_ajp13.method=Session\n" +
	"worker.template_ajp13.socket_connect_timeout=10000\n" +
	"worker.jkstatus.type=status"

func getopt(name, def string) string {
	if env := os.Getenv(name); env != "" {
		return env
	}
	return def
}

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

type TomcatInstance struct {
	HostIp          string
	Internal        bool
	ForceTags       string
	RefreshTtl      int
	RefreshInterval int
	DeregisterCheck string
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(Version)
		os.Exit(0)
	}
	log.Printf("Starting registrator %s ...", Version)

	flag.Parse()

	log.Printf("parsed")
	fmt.Println("hostIp1: ", *hostIp)
	if *hostIp != "" {
		log.Println("Forcing host IP to", *hostIp)
	}
	if (*refreshTtl == 0 && *refreshInterval > 0) || (*refreshTtl > 0 && *refreshInterval == 0) {
		assert(errors.New("-ttl and -ttl-refresh must be specified together or not at all"))
	} else if *refreshTtl > 0 && *refreshTtl <= *refreshInterval {
		assert(errors.New("-ttl must be greater than -ttl-refresh"))
	}

	docker, err := dockerapi.NewClient(getopt("DOCKER_HOST", "unix:///var/run/docker.sock"))
	assert(err)

	// Start event listener before listing containers to avoid missing anything
	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))

	config := consulapi.DefaultConfig()
	config.Address = "192.168.1.125:8500"
	client, res := consulapi.NewClient(config)
	log.Println("Client", client, res)
	agent := client.Agent()
	log.Println("Agent", agent)
	nodename, res := agent.NodeName()
	log.Println("NodeName", nodename, res)
	services, res := agent.Services()
	log.Println("Services", services, services["dude"])

	clusterMap := make(map[string]*consulapi.AgentService)
	tomcatServices := [100]*consulapi.AgentService{}
	i := 0
	for servicename, service := range services {
		log.Println("---------------------------------------------------------")
		log.Println("Servicename: ", servicename)
		log.Println("Address: ", service.Address)
		log.Println("Port: ", service.Port)
		log.Println("Tags: ", service.Tags)
		log.Println("Service: ", service.Service)
		tags := service.Tags
		if stringInSlice("tomcat-service", tags) {
			log.Println("TOMCAT SERVICE!!!!!!!!!!!!!!!")
			clusterMap[service.Service] = service
			tomcatServices[i] = service
			i = i + 1
		}
		log.Println("Service: ", service)

	}
	worker_list := "worker.list=jkstatus"
	log.Println(clusterMap)
	for worker, list := range clusterMap {
		worker_list = worker_list + ",cluster_" + worker
		log.Println("---------------------->", worker, list, worker_list)
	}

	// die einzelnen Loadbalancerkonfigurationen aufbauen
	workersFile := ""
	for worker, _ := range clusterMap {
		workersFile = workersFile + "\nworker.cluster_" + worker + ".type=lb\n"
		workersFile = workersFile + "worker.cluster_" + worker + ".error_escalation_time=0\n"

		cluster_worker := ""
		cluster_worker = "worker.cluster_" + worker + ".balance_workers="

		for key, value := range tomcatServices {
			log.Println(key, value)
			if tomcatServices[key] != nil {
				if tomcatServices[key].Service == worker {
					cluster_worker = cluster_worker + tomcatServices[key].ID + ","
				}
			}
		}
		workersFile = workersFile + cluster_worker
		if strings.HasSuffix(workersFile, ",") {
			workersFile = workersFile[0 : len(workersFile)-1]
		}
	}
	workersFile = workersFile + "\n\n"
	for i, instance := range tomcatServices {
		log.Println(i, instance, tomcatServices[i])
		if instance != nil {
			hostname, key, port, _ := splitServiceName(tomcatServices[i].ID)

			workersFile = workersFile + "worker." + key + ".host=" + hostname + "\n"
			workersFile = workersFile + "worker." + key + ".port=" + port + "\n"
			workersFile = workersFile + "worker." + key + ".reference=worker.template_ajp13 \n"
		}
	}

	log.Println("---------------------------------------------------------")
	log.Println(worker_list)
	log.Println(workersFile)
	log.Println(worker_template)

	//log.Println("getTagValue 123  12345", getTagValue("123", []string {"12345"}))
	//log.Println("getTagValue 123  012345,123abd", getTagValue("123", []string {"012345", "123abd"}))
	//log.Println("getTagValue 123  11aa12345", getTagValue("123", []string {"11aa12345"}))
	log.Println("Listening for Docker events ...")

	quit := make(chan struct{})

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			log.Println("Start event ...")

			nodename, res := agent.NodeName()
			log.Println("NodeName", nodename, res)
			log.Println("Services res", res)

			//for
		case "die":
			log.Println("Die event ...")
		case "stop", "kill":
			log.Println("Stop event ...")
		}
	}

	close(quit)
	log.Fatal("Docker event loop closed") // todo: reconnect?

}

//dude-server:tomcat_dude-server_8217:8009
func splitServiceName(servicename string) (string, string, string, string) {
	result := strings.Split(servicename, ":")

	port := strings.Split(result[1], "_")
	return result[0], result[1], port[2], result[2]
}

func getTagValue(tagname string, taglist []string) string {
	for _, val := range taglist {
		if strings.HasPrefix(val, tagname) {
			return val[len(tagname):]
		}
	}
	return ""
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
