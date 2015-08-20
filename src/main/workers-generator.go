package main

import (
	"container/list"
	"flag"
	"fmt"
	"time"
	dockerapi "github.com/fsouza/go-dockerclient"
	consulapi "github.com/hashicorp/consul/api"
	"log"
	"os"
	"os/exec"
	"strings"
	"path/filepath"
)

var Version string = "v1"

var debug = false
//dockerAddress kann auch unix:/var/run/docker.sock sein

var dockerAddress = flag.String("dockerAddress", "tcp://192.168.1.125:2375", "Address for docker (where events are collected")
var workersDir = flag.String("workersDir", "/usr/local/apache2/conf/", "Pfad where the workers.properties File is written to")
var consulAddress = flag.String("consulAddress", "192.168.1.125:8500", "Address for consul (where information about tomcat instances are gathered")
var reconfigureCommand = flag.String("reconfigureCommand", "/usr/local/apache2/bin/restart.sh", "Optional Command to read the new Configuration created in the workers.properties File")  
var dockerTlsVerifiy = flag.String("DOCKER_TLS_VERIFY", "0", "Use TLS?")
var DOCKER_CERT_PATH = flag.String("DOCKER_CERT_PATH", "/Users/cwoehrle/.boot2docker/certs/boot2docker-vm/", "Path to docker certificates")

var worker_template string = "worker.template_ajp13.type=ajp13\n" +
	"worker.template_ajp13.connection_pool_timeout=300\n" +
	"worker.template_ajp13.connection_pool_minsize=0\n" +
	"worker.template_ajp13.ping_mode=A\n" +
	"worker.template_ajp13.ping_timeout=10000\n" +
	"worker.template_ajp13.lbfactor=10\n" +
	"worker.template_ajp13.activation=A\n" +
	"worker.template_ajp13.recovery_options=7\n" +
	"worker.template_ajp13.retries=2\n" +
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


func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(Version)
		os.Exit(0)
	}
	flag.Parse()
	log.Println("dockerAddress", *dockerAddress)
	dockerconnecttring := getopt("DOCKER_HOST", *dockerAddress)
	log.Println("workersDir", *workersDir)
	log.Println("consulAddress", *consulAddress)
	log.Println("reconfigureCommand", *reconfigureCommand)
	log.Println("dockerTlsVerify", *dockerTlsVerifiy)
	log.Println("DOCKER_CERT_PATH", *DOCKER_CERT_PATH)
	log.Println("docker connectstring", dockerconnecttring)
	
	var docker *(dockerapi.Client)
	var err error
	
	if *dockerTlsVerifiy == "1"  { 
    	log.Println("dockerTlsVerify = 1")
	    fn := filepath.Join(*DOCKER_CERT_PATH, "cert.pem")
	    if _, err := os.Stat("fn"); os.IsNotExist(err) {
			log.Println("File ", fn , " does not exist")
		}
		certFile := fn
		fn = filepath.Join(*DOCKER_CERT_PATH, "key.pem")
	    if _, err := os.Stat("fn"); os.IsNotExist(err) {
			log.Println("File ", fn , " does not exist")
		}
		keyFile := fn
		fn = filepath.Join(*DOCKER_CERT_PATH, "ca.pem")
	    if _, err := os.Stat("fn"); os.IsNotExist(err) {
			log.Println("File ", fn , " does not exist")
		}
		caFile := fn
		
		docker, err = dockerapi.NewTLSClient(dockerconnecttring, certFile, keyFile, caFile) 
	} else {
    	log.Println("dockerTlsVerifiy = 0")
		docker, err = dockerapi.NewClient(getopt("DOCKER_HOST", dockerconnecttring))
	}
	// was macht assert?
	assert(err)


	// Start event listener before listing containers to avoid missing anything
    log.Println("Start Event Listener für Docker Events...")
	events := make(chan *dockerapi.APIEvents)
	assert(docker.AddEventListener(events))

	workersString, res := createWorkers()
	if res == nil {
	    writeFile(*workersDir +"workers.properties", workersString)
    }
	log.Println("Listening for Docker events ...")
	restart()

	quit := make(chan struct{})

	// Process Docker events
	for msg := range events {
		switch msg.Status {
		case "start":
			log.Println("Start event ...")
			log.Println("sleeping 3s ...")
			time.Sleep( 3*time.Second )
			log.Println("sleeping done")
			
			workersString, res := createWorkers()
			if res == nil {
	    		writeFile(*workersDir +"workers.properties", workersString)
    		}
			restart()
			//for
		case "die":
			log.Println("Die event ...")
			workersString, res := createWorkers()
			if res == nil {
	    		writeFile(*workersDir +"workers.properties", workersString)
    		}

		case "stop", "kill":
			log.Println("Stop event ...")
			workersString, res := createWorkers()
			if res == nil {
	    		writeFile(*workersDir +"workers.properties", workersString)
    		}
			default: 
			log.Println("Default Event, was ist denn das?")

		}

	}
	close(quit)
	log.Fatal("Docker event loop closed") // todo: reconnect?

}

// Reads consul information about tomcat instances and returns the workers.properties file
func createWorkers() (string, error) {
	log.Println("createWorkers called to create new workers.properties File")

	config := consulapi.DefaultConfig()
	log.Println("config: ", *config)
	config.Address = *consulAddress
	log.Println("config: ", *config)
	client, res := consulapi.NewClient(config)
	if debug {
		if debug {

			log.Println("Client", client, res)
		}
	}

	agent := client.Agent()
	nodename, res := agent.NodeName()
	log.Println("NodeName", nodename, res)
	services, res := agent.Services()
	log.Println("res", res)
	if res != nil {
		log.Fatal("Error calling consul: ", res )
		log.Fatal("workers.properties is not created!" )
		return "", res
	}

	clusterMap := make(map[string]*consulapi.AgentService)
	//tomcatServices := [100]*consulapi.AgentService{}
	var tomcatList list.List
	for servicename, service := range services {
		if debug {
			log.Println("---------------------------------------------------------")
			log.Println("Servicename: ", servicename)
			log.Println("Address: ", service.Address)
			log.Println("Port: ", service.Port)
			log.Println("Tags: ", service.Tags)
			log.Println("Service: ", service.Service)
		}
		tags := service.Tags
		if stringInSlice("tomcat-service", tags) {
			if debug {
				log.Println("TOMCAT SERVICE!!!!!!!!!!!!!!!")
			}
			clusterMap[service.Service] = service
			//tomcatServices[i] = service
			tomcatList.PushBack(service)
		}
		if debug {
			log.Println("Service: ", service)
		}

	}
	tomcatServices := make([]*consulapi.AgentService, tomcatList.Len())
	for e, i := tomcatList.Front(), 0; e != nil; e, i = e.Next(), i+1 {
		tomcatServices[i] = e.Value.(*consulapi.AgentService)

	}
	worker_list := "worker.list=jkstatus"
	log.Println(clusterMap)
	for worker, list := range clusterMap {
		worker_list = worker_list + ",cluster_" + worker
		if debug {
			log.Println("---------------------->", worker, list, worker_list)
		}
	}

	// die einzelnen Loadbalancerkonfigurationen aufbauen
	workersFile := ""
	for worker, _ := range clusterMap {
		workersFile = workersFile + "\nworker.cluster_" + worker + ".type=lb\n"
		workersFile = workersFile + "worker.cluster_" + worker + ".error_escalation_time=0\n"

		cluster_worker := ""
		cluster_worker = "worker.cluster_" + worker + ".balance_workers="

		for key, value := range tomcatServices {
			if debug {
				log.Println(key, value)
			}

			if tomcatServices[key] != nil {
				if tomcatServices[key].Service == worker {
					_, key, _, _ := splitServiceName(tomcatServices[key].ID)
					cluster_worker = cluster_worker + key + ","
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
		if debug {
			log.Println(i, instance, tomcatServices[i])
		}
		if instance != nil {
			hostname, key, port, _ := splitServiceName(tomcatServices[i].ID)

			workersFile = workersFile + "worker." + key + ".host=" + hostname + "\n"
			workersFile = workersFile + "worker." + key + ".port=" + port + "\n"
			workersFile = workersFile + "worker." + key + ".reference=worker.template_ajp13 \n"
		}
	}
	log.Println("---------------------------------------------------------")
	log.Println(worker_list, workersFile, worker_template)

	//log.Println("getTagValue 123  12345", getTagValue("123", []string {"12345"}))
	//log.Println("getTagValue 123  012345,123abd", getTagValue("123", []string {"012345", "123abd"}))
	//log.Println("getTagValue 123  11aa12345", getTagValue("123", []string {"11aa12345"}))
	return worker_list + workersFile + worker_template, nil
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

func restart( ) int {
	log.Println("Restart WebServer")
	log.Println("Aufruf restart", *reconfigureCommand)
	//args := []string  {"-k", "restart"}
	cmd := exec.Command(*reconfigureCommand )
	err := cmd.Run()
	//_, res := os.StartProcess("/usr/local/apache2/bin/httpd", args, nil)
	log.Println("Aufruf restart", err)
	  
	return 1
}

func writeFile(filename string, content string) int {
	log.Println("writeFile: ", filename)
	file, err := os.Create(filename)
	if err != nil {
		log.Println(err, filename)
		return 1
	}
	defer file.Close()
	_, err = file.WriteString(content)
	log.Println(err, filename)
	
	if err != nil {
		return 2
	}
	return 0
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}

	}
	return false
}