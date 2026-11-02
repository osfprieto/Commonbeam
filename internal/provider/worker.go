package provider

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/osfprieto/Commonbeam/internal/inference"
	"github.com/osfprieto/Commonbeam/internal/network"
	"github.com/osfprieto/Commonbeam/internal/protocol"
)

type Worker struct {
	Client   *network.Client
	Backend  inference.Backend
	Key      ed25519.PrivateKey
	Manifest protocol.Manifest
}

func (w *Worker) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := w.Client.Call(ctx, "POST", "/commonbeam/v1/resume", "", nil, nil); err != nil {
		return err
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		heartbeat := time.NewTicker(5 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				if w.Client.Call(ctx, "POST", "/commonbeam/v1/heartbeat", "", nil, nil) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			w.Client.Call(c, "POST", "/commonbeam/v1/drain", "", nil, nil)
			cancel()
			return nil
		case <-poll.C:
			var out struct {
				Job *protocol.Job `json:"job"`
			}
			if err := w.Client.Call(ctx, "GET", "/commonbeam/v1/work", "", nil, &out); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if out.Job != nil {
				w.execute(ctx, *out.Job)
			}
		}
	}
}

func (w *Worker) execute(parent context.Context, job protocol.Job) {
	ctx, cancel := context.WithDeadline(parent, time.Unix(job.Deadline, 0).Add(-time.Second))
	defer cancel()
	var output string
	var receipt *protocol.SignedReceipt
	var err error
	if job.Request == nil || job.Hash != protocol.RequestCommitment(job.Salt, *job.Request) || job.Request.Model != w.Manifest.Model {
		err = errors.New("invalid assigned request")
	} else {
		var usage protocol.Usage
		usage, err = w.Backend.Generate(ctx, *job.Request, func(text string) error {
			if len(output)+len(text) > 32768 {
				return errors.New("output exceeds test policy")
			}
			if err := w.Client.Call(ctx, "POST", "/commonbeam/v1/work/"+job.ID+"/append", "", map[string]string{"text": text}, nil); err != nil {
				return err
			}
			output += text
			return nil
		})
		if err == nil {
			r := protocol.SignReceipt(protocol.Receipt{
				Version: protocol.Version, Network: w.Manifest.Network, Job: job.ID, RequestHash: job.Hash,
				Provider: w.Manifest.Provider, Model: w.Manifest.Model, Profile: w.Manifest.Profile,
				OutputHash: protocol.OutputCommitment(job.Salt, output), Usage: usage, Simulation: w.Manifest.Backend == "fake",
			}, w.Key)
			receipt = &r
		}
	}
	c, finish := context.WithTimeout(context.Background(), 4*time.Second)
	defer finish()
	// A lost completion is reconciled by the authority deadline; it is never re-executed silently.
	w.Client.Call(c, "POST", "/commonbeam/v1/work/"+job.ID+"/finish", "", map[string]any{"receipt": receipt}, nil)
}
