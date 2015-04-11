# docker-modjk-bridge

This project reads the service definitions of a tomcat cluster from consul and creates a workers.properties file. 

The workers.properties file is for the mod\_jk module of the apache http server. mod\_jk uses this file to distribute requests across the tomcat cluster. 

The code is triggered when a docker container is started or stopped. It checks the service configuration in consul to find all the tomcat containers with the tag `tomcat_service`.


I reused a lot of the code from  gliderlabs/registrator to catch events to access consul. This is a great place to learn about go programming. 

