#!/bin/bash
cat << EOF | sudo tee /opt/impala/conf/impala.env
export IMPALA_HOME=/opt/impala
export JAVA_HOME=/usr/lib/jvm/java/
export CLASSPATH=/opt/impala/lib/*:/opt/hive/lib/*
export HADOOP_HOME=/opt/hadoop
export HIVE_HOME=/opt/hive
export METASTORE_HOME=/opt/hive
export HIVE_CONF=/opt/hive/conf
EOF
sudo chown impala: /opt/impala/conf/impala.env
source /opt/impala/conf/impala.env
source /etc/environments/impala.env
./propgen -label IMPALA -render impaladaemon -file /opt/impala/conf/impala.gflagfile
sudo chown impala: /opt/impala/conf/impala.gflagfile

./propgen -label IMPALA -render impalacatalog -file /opt/impala/conf/catalog.gflagfile
sudo chown impala: /opt/impala/conf/catalog.gflagfile

./propgen -label IMPALA -render impalastatestore -file /opt/impala/conf/statestore.gflagfile
sudo chown impala: /opt/impala/conf/statestore.gflagfile

./propgen -label IMPALA -render impalaadmission -file /opt/impala/conf/admission.gflagfile
sudo chown impala: /opt/impala/conf/admission.gflagfile

./propgen -label IMPALA -render hivesite -file /opt/hive/conf/hive-site.xml

./propgen -label IMPALA -render admissiondaemon -file /opt/hadoop/etc/hadoop/core-site.xml

./propgen -label IMPALA -render hdfssite -file /opt/hadoop/etc/hadoop/hdfs-site.xml
