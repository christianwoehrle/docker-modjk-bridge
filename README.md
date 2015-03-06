# docker-modjk-bridge
Part of a bigger configuration.

Needs Consul to store information about conatainers

Needs gliderlabs/registrator to catch events and store them in consul

This bridge gets events when a docker container is started, checks the configuration in consul to find all the tomcat containers  and creates the workers.properties file so that Apache with mod_jk can update its cluster configuration.


