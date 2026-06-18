package worker

import "github.com/hibiken/asynq"

// NewServer builds the asynq server and a mux routing each task type to its
// handler. The worker binary calls srv.Run(mux).
func NewServer(redisAddr string, p *Processor) (*asynq.Server, *asynq.ServeMux) {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{Concurrency: 10},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeWebhookProcess, p.HandleWebhookProcess)
	return srv, mux
}
