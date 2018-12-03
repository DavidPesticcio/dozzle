package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"io"
	"log"
	"sort"
	"strings"
)

type dockerClient struct {
	cli *client.Client
}

// Client is a proxy around the docker client
type Client interface {
	ListContainers() ([]Container, error)
	ContainerLogs(ctx context.Context, id string) (<-chan string, <-chan error)
	Events(ctx context.Context) (<-chan events.Message, <-chan error)
}

// NewClient creates a new instance of Client
func NewClient() Client {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}
	return &dockerClient{cli}
}

func (d *dockerClient) ListContainers() ([]Container, error) {
	list, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}

	var containers []Container
	for _, c := range list {

		container := Container{
			ID:      c.ID[:12],
			Names:   c.Names,
			Name:    strings.TrimPrefix(c.Names[0], "/"),
			Image:   c.Image,
			ImageID: c.ImageID,
			Command: c.Command,
			Created: c.Created,
			State:   c.State,
			Status:  c.Status,
		}
		containers = append(containers, container)
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	if containers == nil {
		containers = []Container{}
	}

	return containers, nil
}

func (d *dockerClient) ContainerLogs(ctx context.Context, id string) (<-chan string, <-chan error) {
	options := types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "300", Timestamps: true}

	reader, err := d.cli.ContainerLogs(ctx, id, options)
	if err != nil {
		tmpErrors := make(chan error, 1)
		tmpErrors <- err
		return nil, tmpErrors
	}

	go func() {
		<-ctx.Done()
		reader.Close()
	}()

	messages := make(chan string)
	errChannel := make(chan error)

	go func() {
		hdr := make([]byte, 8)
		var buffer bytes.Buffer
		for {
			_, err := reader.Read(hdr)
			if err != nil {
				errChannel <- err
				break
			}
			count := binary.BigEndian.Uint32(hdr[4:])
			_, err = io.CopyN(&buffer, reader, int64(count))
			if err != nil {
				errChannel <- err
				break
			}
			messages <- buffer.String()
			buffer.Reset()
		}
		close(messages)
		close(errChannel)
		reader.Close()
	}()

	return messages, errChannel

}

func (d *dockerClient) Events(ctx context.Context) (<-chan events.Message, <-chan error) {
	return d.cli.Events(ctx, types.EventsOptions{})
}
