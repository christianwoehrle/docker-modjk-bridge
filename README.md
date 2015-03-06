# docker-modjk-bridge
The bridge gets events when a docker container is started, checks if the container is relavant for a tomcat cluster and creates the workers.properties file so that Apache with mod_jk can update its cluster configuration
