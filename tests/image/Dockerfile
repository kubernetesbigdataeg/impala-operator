FROM centos:centos7

COPY impala-4.1.0-SNAPSHOT.tar.gz /
RUN yum -y install sudo
RUN sudo yum install -y java-1.8.0-openjdk-devel && \
    yum install -y gcc python-develyum install cyrus-sasl*

RUN sudo adduser impala && \
  sudo mkdir /var/log/impala && \
  sudo chown -R impala: /var/log/impala && \
  sudo mkdir -p /opt/impala/conf && \
  sudo chown -R impala: /opt/impala/conf

RUN sudo tar xzf impala-4.1.0-SNAPSHOT.tar.gz -P -C /
RUN sudo chown -R impala: /opt/impala/
RUN rm -r /impala-4.1.0-SNAPSHOT.tar.gz

RUN mv /opt/impala/lib64/libkudu_client.so.0.1.0 /opt/impala/lib64/libkudu_client.so.0
RUN mv /opt/impala/lib64/libstdc++.so.6.0.24 /opt/impala/lib64/libstdc++.so.6
RUN sudo echo '/opt/impala/lib64' | tee /etc/ld.so.conf.d/impala.conf

COPY docker-entrypoint.sh impala-config.sh /

#RUN curl -s https://jdbc.postgresql.org/download/postgresql-42.2.16.jar -o postgresql-42.2.16.jar && mv postgresql-42.2.16.jar /opt/hive/lib/postgresql-jdbc.jar

#RUN sudo adduser -r hive
#RUN sudo chown -R hive: /opt/hive/
#COPY hms-plugin.jar /opt/hive/lib/
#RUN sudo chown hive: /opt/hive/lib/hms-plugin.jar

RUN sudo chmod 777 docker-entrypoint.sh impala-config.sh

#USER impala

ENTRYPOINT ["/docker-entrypoint.sh"]
EXPOSE 8050 25010 25020 25030