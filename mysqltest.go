package mysqltest

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type MysqldConfig struct {
	Tag     string
	Timeout time.Duration
}

func NewMysqldConfig() *MysqldConfig {
	return &MysqldConfig{
		Tag:     "mysql:latest",
		Timeout: 30,
	}
}

type Mysqld struct {
	port      string
	host      string
	config    *MysqldConfig
	container string
}

func NewMysqld(config *MysqldConfig) (*Mysqld, error) {
	if config == nil {
		config = NewMysqldConfig()
	}
	mysqld := &Mysqld{
		config: config,
	}
	if err := mysqld.start(); err != nil {
		return nil, err
	}
	return mysqld, nil
}

func (m *Mysqld) DSN() string {
	return fmt.Sprintf("%s@tcp(%s:%s)/%s", "root", m.host, m.port, "test")
}

func (m *Mysqld) Stop() {
	killCointainer(m.container)
	removeContainer(m.container)
}

func (m *Mysqld) start() error {
	cmd, err := m.dockerRunCommand()
	if err != nil {
		return err
	}
	_container, err := cmd.Output()
	container := string(chomp(_container))
	if err != nil {
		return err
	}

	m.container = container

	if inDockerContainer() {
		o, err := exec.Command("docker", "inspect", "--format={{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", m.container).Output()
		if err != nil {
			return err
		}
		m.host = string(chomp(o))
		m.port = "3306"
	} else {
		o, err := exec.Command("docker", "inspect", "--format={{range $p, $conf := .NetworkSettings.Ports}}{{ if eq $p \"3306/tcp\" }}{{(index $conf 0).HostPort}}{{end}}{{end}}", m.container).Output()
		if err != nil {
			fmt.Printf("%#v\n", err)
			return err
		}
		m.host = "127.0.0.1"
		m.port = string(chomp(o))
	}

	timeout := time.NewTimer(time.Second * m.config.Timeout)
	connect := time.NewTicker(time.Second)
	dsn := m.DSN()

	for {
		select {
		case <-timeout.C:
			killCointainer(container)
			removeContainer(container)
			return fmt.Errorf("timeout: failed to connect mysqld")
		case <-connect.C:
			db, err := sql.Open("mysql", dsn)
			if err != nil {
				return err
			}
			if err := db.Ping(); err != nil {
				continue
			}
			return nil
		}
	}
}

func (m *Mysqld) dockerRunCommand() (*exec.Cmd, error) {
	var args = []string{"run"}
	if inDockerContainer() {
		o, err := exec.Command("docker", "inspect", "--format={{.HostConfig.NetworkMode}}", os.Getenv("HOSTNAME")).Output()
		if err != nil {
			return nil, err
		}
		network := chomp(o)
		args = append(args, "--network", string(network))
	} else {
		args = append(args, "-p", ":3306")
	}
	args = append(args, "-e", "MYSQL_ALLOW_EMPTY_PASSWORD=1")
	args = append(args, "-e", "MYSQL_DATABASE=test")
	args = append(args, "-d", m.config.Tag)
	return exec.Command("docker", args...), nil
}

func inDockerContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func killCointainer(id string) error {
	return exec.Command("docker", "kill", id).Run()
}

func removeContainer(id string) error {
	return exec.Command("docker", "rm", "-v", id).Run()
}

func chomp(v []byte) []byte {
	return bytes.TrimRight(v, "\n")
}
