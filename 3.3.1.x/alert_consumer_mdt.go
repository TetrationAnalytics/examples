package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"io"
	"archive/tar"
	"compress/gzip"
	"github.com/golang/glog"
	"errors"
	"io/ioutil"
	"bufio"
	"strings"
	"log"
	"sync"
	"time"

	"crypto/tls"
	"crypto/x509"

	"github.com/Shopify/sarama"
)

const (
	ConsumerCertificateFile string = "KafkaConsumerCA.cert"
	ConsumerPrivateKeyFile  string = "KafkaConsumerPrivateKey.key"
	RootCAFile              string = "KafkaCA.cert"
	KafkaCAFile             string = "KafkaCA.cert"
	KafkaBrokersFile        string = "kafkaBrokerIps.txt"
	TopicFile               string = "topic.txt"
)

var kafkaClient sarama.Client
var kafkaConsumer sarama.Consumer

// untar tar gz file into the directory
func untarTarGzFile(dir string, file io.Reader) error {
	glog.Info("Creating dir ", dir)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		glog.Error("Create of dir failed name ", dir, err)
		return err
	}
	gzf, err := gzip.NewReader(file)
	if err != nil {
		glog.Error(fmt.Sprintf("Error unzipping/untaring file %s: %s", dir, err.Error()))
		return errors.New("Untar error")
	}
	defer gzf.Close()

	tarReader := tar.NewReader(gzf)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			glog.Error(fmt.Sprintf("Error untaring file %s: %s", dir, err.Error()))
			return errors.New("Untar error")
		}
		switch header.Typeflag {
		case tar.TypeDir: // directory
			target := filepath.Join(dir, header.Name)
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0666); err != nil {
					return err
				}
			}
		case tar.TypeRegA:
			fallthrough
		case tar.TypeReg: // regular file
			glog.Info(fmt.Sprintf("Trying to tar file %s", header.Name))
			target := filepath.Join(dir, header.Name)
			fw, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				glog.Error("Failed to create ", target, err.Error())
				return err
			}
			defer fw.Close()
			if err != nil {
				// file create failed, return error
				glog.Error("Failed to create certs file", err.Error())
				return errors.New("Failed to create certs file")
			}
			// write file to hdfs writer
			written, err := io.Copy(fw, tarReader)
			if err != nil {
				glog.Error(fmt.Sprintf("Error when uploading %s: %+v", dir, err))
				fw.Close()
				return errors.New("ACS file create/upload error")
			}
			glog.Info(fmt.Sprintf("Uploaded %d bytes to %s", written, dir))

		default:
			continue
		}
	}
	return nil
}

func main() {
	var keystore_location string
	flag.StringVar(&keystore_location, "keystore_location", "", "a string var")
	flag.Parse()
	if (keystore_location == "") {
		fmt.Println("USAGE: ./<binary_name> -keystore_location=<file_on_local_filesystem>")
		return
	}
	fmt.Printf("Keystore_Location = %s\n", keystore_location)

	// Check if keystore location actually exists
	if _, err := os.Stat(keystore_location); err != nil {
		fmt.Printf("The keystore file %s does not exist\n", keystore_location)
        return
	}

	// Create a keystores directory in the CWD (cur working dir)
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
    if err != nil {
		fmt.Errorf("Could not get the current directory")
		return
    }

	// Create a temporary certificate directory.
	kestore_directory := fmt.Sprintf("%s/%s", currentDir, "keystores")
	os.MkdirAll(kestore_directory, 0755)

	// Open the file handle
	file, err := os.Open(keystore_location)
	if err != nil {
		fmt.Printf("Could not open the file handle\n")
		return
	}
	// Create a Reader and untar the tar.gz file in local filesystem
	reader := bufio.NewReader(file)
	err = untarTarGzFile(kestore_directory, reader)
	if err != nil {
		fmt.Println("Untarring failed")
        return
	}

	// Read Brokers File and Topics File
	kafkaBrokersFile := filepath.Join(kestore_directory, KafkaBrokersFile)
	topicFile := filepath.Join(kestore_directory, TopicFile)

	// Get the list of brokers from the BrokerFile
	b, err := ioutil.ReadFile(kafkaBrokersFile)
	if err != nil {
		fmt.Printf("Failed to read brokers file\n")
		return
	}
	brokers := strings.Split(string(b), ",")
	fmt.Printf("Connecting to broker %s\n", brokers[0])

	// Get Topic
	t, err := ioutil.ReadFile(topicFile)
	if err != nil {
		fmt.Printf("Could not read the topics file")
		return
	}
	topicMdt := string(t)
	fmt.Printf("Topic Name=%s\n", topicMdt)

	// CONFIG
	config := sarama.NewConfig()
	config.ClientID = "TryGoConsumer"
	config.Consumer.Return.Errors = true

	// TLS Settings
	config.Net.TLS.Enable = true
	config.Net.TLS.Config = &tls.Config{}
	// set TLS version to TLSv1.2
	config.Net.TLS.Config.MinVersion = tls.VersionTLS12
	config.Net.TLS.Config.MaxVersion = tls.VersionTLS12
	config.Net.TLS.Config.PreferServerCipherSuites = true
	config.Net.TLS.Config.InsecureSkipVerify = true

	fmt.Printf("MinVersion: %d, MaxVersion: %d\n",
		config.Net.TLS.Config.MinVersion,
		config.Net.TLS.Config.MaxVersion)

	// Load ClientCertificateFile and ClientPrivateKeyFile and pass it here
	ClientCertificateFile := filepath.Join(kestore_directory, ConsumerCertificateFile)
	ClientPrivateKeyFile := filepath.Join(kestore_directory, ConsumerPrivateKeyFile)
	RootCA_File := filepath.Join(kestore_directory, RootCAFile)

	// Load the KeyPair
	cert, err := tls.LoadX509KeyPair(ClientCertificateFile, ClientPrivateKeyFile)
	if err != nil {
		fmt.Errorf("Error loading certificats: ", err)
		return
	}
	config.Net.TLS.Config.Certificates = []tls.Certificate{cert}

	// Add Root CA intp this CertPool
	tlsCertPool := x509.NewCertPool()
	caCertFile, err := ioutil.ReadFile(RootCA_File)
	tlsCertPool.AppendCertsFromPEM(caCertFile)
	config.Net.TLS.Config.RootCAs = tlsCertPool

	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Compression = sarama.CompressionSnappy
	config.Producer.Flush.Frequency = 500 * time.Millisecond

	// Create Kafka Client
	kafkaClient, err = sarama.NewClient(brokers, config)
	if err != nil {
		fmt.Printf("Failed to connect to Kafka (%s)\n", err.Error())
		return
	}
	// Create Kafka Consumer
	kafkaConsumer, err = sarama.NewConsumerFromClient(kafkaClient)
	if err != nil {
		fmt.Printf("Failed to create consumer (%s)\n", err.Error())
		return
	}

	// Go Routine to Consume Data
	go func() {
		wg := sync.WaitGroup{}
		for {
			partitions, _ := kafkaConsumer.Partitions(topicMdt)
			for _, partition := range partitions {
				consumer, err := kafkaConsumer.ConsumePartition(topicMdt, partition, sarama.OffsetOldest)
				if err != nil {
					log.Panic(fmt.Errorf("Failure to consume partition %d (%s)", partition, err))
				}
				wg.Add(1)
				go func(c sarama.PartitionConsumer) {
					for {
						select {
						case m := <-c.Messages():
							if m == nil {
								log.Println("Consumer is dead - reconnect")
								wg.Done()
								return
							}
							fmt.Println("Received messages", string(m.Key), string(m.Value))
						}
					}
					wg.Done()
				}(consumer)
			}
			wg.Wait() // Cycle around if all goroutines broke out
		}
	}()

	log.Println("Registered consumers for all required topics and partitions...waiting for messages")
	// Wait forever and check the status of all the consumers periodically
	for {
		time.Sleep(1 * time.Second)
		_, err = kafkaConsumer.Topics()
		if err != nil {
			log.Panic("Failed to list topics on Kafka broker")
		}
	}
}
