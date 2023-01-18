#!/bin/bash
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
