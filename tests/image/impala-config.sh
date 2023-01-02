#!/bin/bash
cat << EOF | sudo tee /opt/impala/conf/impala.env
IMPALA_HOME=/opt/impala
JAVA_HOME=/usr/lib/jvm/java/
CLASSPATH=/opt/impala/lib/*:/opt/hive/lib/*
HADOOP_HOME=/opt/hadoop
HIVE_HOME=/opt/hive
HIVE_CONF=/opt/hive/conf
EOF
sudo chown impala: /opt/impala/conf/impala.env
cat << EOF | sudo tee /opt/impala/conf/impala.gflagfile
--abort_on_config_error=false
--log_dir=/var/log/impala
--state_store_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
--catalog_service_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
--admission_service_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
--kudu_master_hosts=kudu-master-0.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-1.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-2.kudu-masters.kudu.svc.cluster.local:7051
--enable_legacy_avx_support=true
EOF
sudo chown impala: /opt/impala/conf/impala.gflagfile
cat << EOF | sudo tee /opt/impala/conf/catalog.gflagfile
--kudu_master_hosts=kudu-master-0.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-1.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-2.kudu-masters.kudu.svc.cluster.local:7051
--log_dir=/var/log/impala
--enable_legacy_avx_support=true
EOF
sudo chown impala: /opt/impala/conf/catalog.gflagfile
cat << EOF | sudo tee /opt/impala/conf/statestore.gflagfile
--kudu_master_hosts=kudu-master-0.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-1.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-2.kudu-masters.kudu.svc.cluster.local:7051
--log_dir=/var/log/impala
--enable_legacy_avx_support=true
EOF
sudo chown impala: /opt/impala/conf/statestore.gflagfile
cat << EOF | sudo tee /opt/impala/conf/admission.gflagfile
--kudu_master_hosts=kudu-master-0.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-1.kudu-masters.kudu.svc.cluster.local:7051,kudu-master-2.kudu-masters.kudu.svc.cluster.local:7051
--log_dir=/var/log/impala
--enable_legacy_avx_support=true
EOF
sudo chown impala: /opt/impala/conf/admission.gflagfile
cat << EOF | sudo tee /opt/hive/conf/hive-site.xml
<configuration>
  <property>
      <name>datanucleus.autoCreateSchema</name>
      <value>false</value>
  </property>
  <property>
      <name>hive.metastore.uris</name>
      <value>thrift://hive-metastore-svc.kudu.svc.cluster.local:9083</value>
  </property>
  <property>
      <name>hive.metastore.warehouse.dir</name>
      <value>/var/lib/hive/warehouse</value>
  </property>
  <property>
      <name>javax.jdo.option.ConnectionDriverName</name>
      <value>org.postgresql.Driver</value>
  </property>
  <property>
      <name>javax.jdo.option.ConnectionPassword</name>
      <value>postgres</value>
  </property>
  <property>
      <name>javax.jdo.option.ConnectionURL</name>
      <value>jdbc:postgresql://postgresql-svc.kudu.svc.cluster.local:5432/metastore</value>
  </property>
  <property>
      <name>javax.jdo.option.ConnectionUserName</name>
      <value>postgres</value>
  </property>
  <property>
      <name>metastore.expression.proxy</name>
      <value>org.apache.hadoop.hive.metastore.DefaultPartitionExpressionProxy</value>
  </property>
  <property>
      <name>metastore.task.threads.always</name>
      <value>org.apache.hadoop.hive.metastore.events.EventCleanerTask,org.apache.hadoop.hive.metastore.MaterializationsCacheCleanerTask</value>
  </property>
    <property>
    <name>hive.metastore.transactional.event.listeners</name>
    <value>      
      org.apache.hive.hcatalog.listener.DbNotificationListener,
      org.apache.kudu.hive.metastore.KuduMetastorePlugin
    </value> 
  </property> 
  <property>
    <name>hive.metastore.disallow.incompatible.col.type.changes</name>
    <value>false</value>
  </property>
  <property>
      <name>hive.metastore.dml.events</name>
      <value>true</value>
  </property>
  <property>
     <name>hive.metastore.event.db.notification.api.auth</name>
     <value>false</value>
  </property>
</configuration>
EOF
cat << EOF | sudo tee /opt/hadoop/etc/hadoop/core-site.xml
<?xml version="1.0"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
  <property>
      <name>fs.defaultFS</name>
      <value>hdfs://hdfs-k8s</value>
  </property>
  <property>
      <name>ha.zookeeper.quorum</name>
      <value>zk-0.zk-hs.default.svc.cluster.local:2181,zk-1.zk-hs.default.svc.cluster.local:2181,zk-2.zk-hs.default.svc.cluster.local:2181</value>
  </property>
</configuration>
EOF
cat << EOF | sudo tee /opt/hadoop/etc/hadoop/hdfs-site.xml
<?xml version="1.0"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<configuration>
  <property>
      <name>dfs.client.failover.proxy.provider.hdfs-k8s</name>
      <value>org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider</value>
  </property>

  <property>
      <name>dfs.datanode.data.dir</name>
      <value>/hadoop/dfs/data</value>
  </property>

  <property>
      <name>dfs.ha.automatic-failover.enabled</name>
      <value>true</value>
  </property>

  <property>
      <name>dfs.ha.fencing.methods</name>
      <value>shell(/bin/true)</value>
  </property>

  <property>
      <name>dfs.ha.namenodes.hdfs-k8s</name>
      <value>nn0,nn1</value>
  </property>

  <property>
      <name>dfs.journalnode.edits.dir</name>
      <value>/hadoop/dfs/journal</value>
  </property>

  <property>
      <name>dfs.namenode.datanode.registration.ip-hostname-check</name>
      <value>false</value>
  </property>

  <property>
      <name>dfs.namenode.http-address.hdfs-k8s.nn0</name>
      <value>hdfs-namenode-0.hdfs-namenode-svc.default.svc.cluster.local:50070</value>
  </property>

  <property>
      <name>dfs.namenode.http-address.hdfs-k8s.nn1</name>
      <value>hdfs-namenode-1.hdfs-namenode-svc.default.svc.cluster.local:50070</value>
  </property>

  <property>
      <name>dfs.namenode.name.dir</name>
      <value>file:///hadoop/dfs/name</value>
  </property>

  <property>
      <name>dfs.namenode.rpc-address.hdfs-k8s.nn0</name>
      <value>hdfs-namenode-0.hdfs-namenode-svc.default.svc.cluster.local:8020</value>
  </property>

  <property>
      <name>dfs.namenode.rpc-address.hdfs-k8s.nn1</name>
      <value>hdfs-namenode-1.hdfs-namenode-svc.default.svc.cluster.local:8020</value>
  </property>

  <property>
      <name>dfs.namenode.shared.edits.dir</name>
      <value>qjournal://hdfs-journalnode-0.hdfs-journalnode-svc.default.svc.cluster.local:8485;hdfs-journalnode-1.hdfs-journalnode-svc.default.svc.cluster.local:8485;hdfs-journalnode-2.hdfs-journalnode-svc.default.svc.cluster.local:8485/hdfs-k8s</value>
  </property>

  <property>
      <name>dfs.nameservices</name>
      <value>hdfs-k8s</value>
  </property>

  <property>
    <name>dfs.client.read.shortcircuit</name>
    <value>true</value>
  </property>

  <property>
      <name>dfs.domain.socket.path</name>
      <value>/var/run/hdfs-sockets/dn</value>
  </property>

  <property>
      <name>dfs.client.file-block-storage-locations.timeout.millis</name>
      <value>10000</value>
  </property>

  <property>
    <name>dfs.datanode.hdfs-blocks-metadata.enabled</name>
    <value>true</value>
  </property>

</configuration>
EOF