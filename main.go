package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config/targetgroup"
	"github.com/prometheus/prometheus/documentation/examples/custom-sd/adapter"
)

// Client struct for registry client to file_sd
type Client struct {
	Targets []string `json:"targets" binding:"required"`
	Labels  struct {
		InstanceName string `json:"instance_name,omitempty"`
		App          string `json:"app" binding:"required"`
		MetricsPath  string `json:"metrics_path,omitempty"`
	} `json:"labels app" binding:"required"`
}

//Config function
type sdConfig struct {
	RefreshInterval int
}

//Config function
type Config struct {
	refreshInterval int
	logger          log.Logger
	oldSourceList   map[string]bool
}

var (
	client Client
	logger log.Logger
)

// JSONError application json format response for error
func JSONError(w http.ResponseWriter, err interface{}, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(err)
}

func (c Client) readClient(w http.ResponseWriter, req *http.Request) {

	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	err := decoder.Decode(&c)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		fmt.Println(c, err.Error())
		return
	}
	fields := reflect.ValueOf(&c).Elem()
	for i := 0; i < fields.NumField(); i++ {
		labelRequired := fields.Type().Field(i).Tag.Get("binding")
		labelValue := fields.Type().Field(i).Tag.Get("json")
		if strings.Contains(labelRequired, "required") && fields.Field(i).IsZero() {
			JSONError(w, fmt.Sprintf("`%v`: e obrigatorio", labelValue), 400)
			fmt.Println("Field", labelRequired, labelValue)
			return
		}

	}
	fmt.Println(c.Targets)
}

func (conf *Config) parseServiceNodes(metric map[string]string, name string) (*targetgroup.Group, error) {

	tgroup := targetgroup.Group{
		Source: name,
		Labels: make(model.LabelSet),
	}

	tgroup.Targets = make([]model.LabelSet, 0, len(metric))
	instance := strings.Split(metric["instance"], ":")[0] + ":" + metric["exporter_port"]
	tgroup.Source = instance
	target := model.LabelSet{model.AddressLabel: model.LabelValue(instance)}
	for k, v := range metric {
		if k == "__name__" {
			tgroup.Labels[model.LabelName(k)] = model.LabelValue(v)
			continue
		}
		tgroup.Labels[model.LabelName(model.MetaLabelPrefix+k)] = model.LabelValue(v)
	}
	tgroup.Targets = append(tgroup.Targets, target)

	return &tgroup, nil
}

//Run function
func (conf *Config) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	for c := time.Tick(time.Duration(60) * time.Second); ; {

		var tgs []*targetgroup.Group

		name := "teste-conf-name"
		newSourceList := make(map[string]bool)

		var metrics *Client

		for _, v := range metrics {

			tg, err := conf.parseServiceNodes(v.Metric, name)
			if err != nil {
				level.Error(conf.logger).Log("msg", "Error parsing metrics", "service", name, "err", err)
				break
			}
			tgs = append(tgs, tg)
			newSourceList[tg.Source] = true
		}
		// When targetGroup disappear, send an update with empty targetList.
		for key := range conf.oldSourceList {
			if !newSourceList[key] {
				tgs = append(tgs, &targetgroup.Group{
					Source: key,
				})
			}
		}
		conf.oldSourceList = newSourceList
		if err == nil {
			// We're returning all exporters as a single targetgroup.
			ch <- tgs
		}
		// Wait for ticker or exit when ctx is closed.
		select {
		case <-c:
			continue
		case <-ctx.Done():
			return
		}
	}
}
func newDiscovery(conf sdConfig) (*Config, error) {
	cd := &Config{
		refreshInterval: conf.RefreshInterval,
		logger:          logger,
		oldSourceList:   make(map[string]bool),
	}
	return cd, nil
}

func main() {
	logger = log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	ctx := context.Background()

	cfg := sdConfig{
		RefreshInterval: 30,
	}

	disc, err := newDiscovery(cfg)
	if err != nil {
		fmt.Println("err: ", err)
	}

	sdAdapter := adapter.NewAdapter(ctx, "/opt/file_sd/teste-metrics.json", "autogenarate_sd", disc, logger)
	sdAdapter.Run()

	http.HandleFunc("/client", client.readClient)
	level.Error(http.ListenAndServe(":8082", nil))
}
