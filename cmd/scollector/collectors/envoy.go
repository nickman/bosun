package collectors

import (
	"bosun.org/cmd/scollector/conf"
	"net/http"
	"bosun.org/opentsdb"
	"bosun.org/metadata"
	"strings"
	"errors"
	"io/ioutil"

	"fmt"
	"regexp"
)

const COUNTER = "counter"
const GAUGE = "gauge"
var whiteSpaceSplitter = regexp.MustCompile("\\s+")
var commaSplitter = regexp.MustCompile(",")
var metricLinePattern = regexp.MustCompile("(?P<metricname>.*?)\\{(?P<tags>.*?)\\}\\s+(?P<value>.*)$")
var tagPairPattern = regexp.MustCompile("(?P<key>.*?)\\=\"(?P<value>.*?)\"$")
// e.g. envoy_http_tracing_service_forced{envoy_http_conn_manager_prefix="ingress_http"} 0



var (
	envoyStatsURL = "/stats?format=prometheus"
	envoyClustersURL = "/clusters"
)

func init() {
	registerInit(func(c *conf.Conf) {
		host := ""
		if c.EnvoyHost != "" {
			host = "http://" + c.EnvoyHost
		} else {
			host = "http://localhost:9901"
		}
		envoyStatsURL = host + envoyStatsURL
		envoyClustersURL = host + envoyClustersURL
		collectors = append(collectors, &IntervalCollector{F: c_envoy_stats, Enable: enableURL(envoyStatsURL)})
	})
}

func getLines(url string) (error, []string) {
	res, err := http.Get(url)
	if err != nil {
		return err, nil
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		bodyBytes, err2 := ioutil.ReadAll(res.Body)
		if err2 != nil {
			return err2, nil
		}
		bodyString := string(bodyBytes)
		return nil, strings.Split(bodyString, "\n")
	}
	return errors.New(fmt.Sprintf("Error %d retrieving URL: %s", res.StatusCode, url)), nil
}


func extractMeta(line string) (metadata.RateType, string) {
	fields := whiteSpaceSplitter.Split(line, -1)
	t := fields[3]
	switch t {
	case COUNTER:
		return metadata.Counter, fields[2]
	case GAUGE:
		return metadata.Gauge, fields[2]
	default:
		return metadata.Counter, fields[2]
	}
}

func parse(value string, r *regexp.Regexp) map[string]string {
	match := r.FindStringSubmatch(value)
	result := make(map[string]string)
	for i, name := range r.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	return result
}

func parseTags(f map[string]string) map[string]string {
	result := make(map[string]string)
	tagtext := f["tags"]
	tagPairs := commaSplitter.Split(tagtext, -1)
	for _, tagPair := range tagPairs {
		tag := parse(tagPair, tagPairPattern)
		result[tag["key"]] = tag["value"]
	}
	return result
}


func extractMetric(line string) (string, opentsdb.TagSet, string) {  // metricName, tags, value
	fields := parse(line, metricLinePattern)
	tags := parseTags(fields)
	return fields["metricname"], tags, fields["metricname"]
}

func c_envoy_stats() (opentsdb.MultiDataPoint, error) {
	err, lines := getLines(envoyStatsURL)

	if err != nil {
		return nil, err
	}
	var md opentsdb.MultiDataPoint
	var metricType metadata.RateType
	var metricDesc string
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			metricType, metricDesc = extractMeta(line)
		} else {
			name, tags, value := extractMetric(line)
			Add(&md, name, value, tags, metricType, metadata.None, metricDesc)
			metricType = ""
			metricDesc = ""
		}
	}
	return md, nil
}


