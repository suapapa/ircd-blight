package ircd

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"strconv"
	"strings"
)

// a Password stores Passwords for Oper and User directives.
type Password struct {
	Type     string `json:"type"`
	Password string `json:"pass"`
}

// An Oper is an operator configuration directive.
type Oper struct {
	Name     string    `json:"name"`
	Password *Password `json:"password"`
	Host     []string  `json:"hosts"`
	Flag     []string  `json:"flags"`
}

// A Class is a user/server connection class directive.
type Class struct {
	Name string   `json:"name"`
	Host []string `json:"hosts"`
	Flag []string `json:"flags"`
}

// A Link represents the configuration information for a remote
// server link.
type Link struct {
	Name string   `json:"name"`
	Host []string `json:"hosts"`
	Flag []string `json:flags"`
}

// A Ports direcive stores a port range and whether or not it is an SSL port.
type Ports struct {
	SSL        bool   `json:"ssl"`
	PortString string `json:"port"`
}

// GetPortList gets the port list specified by the range(s) in this ports directive.
// The following port range formats are understood:
//   6667           // A single port
//   6666-6669      // A port range
//   6666-6669,6697 // Comma-separated ranges
func (p *Ports) GetPortList() (ports []int, err error) {
	ranges := strings.Split(p.PortString, ",")
	for _, rng := range ranges {
		extremes := strings.Split(strings.TrimSpace(rng), "-")
		if len(extremes) > 2 {
			return nil, errors.New("Invalid port range: " + rng)
		}
		low, err := strconv.Atoi(extremes[0])
		if err != nil {
			return nil, err
		}
		if len(extremes) == 1 {
			ports = append(ports, low)
			continue
		}
		high, err := strconv.Atoi(extremes[1])
		if err != nil {
			return nil, err
		}
		if low > high {
			return nil, errors.New("Inverted range: " + rng)
		}
		for port := low; port <= high; port++ {
			ports = append(ports, port)
		}
	}
	return
}

// A Network represents the configuration data for the network on which
// this server is running.
type Network struct {
	Name        string  `json:"name"`
	Description string  `json:"desc"`
	Link        []*Link `json:"links"`
}

// A Configuration stores the configuration information for this server.
type Configuration struct {
	Name     string   `json:"name"`
	SID      string   `json:"sid"`
	Admin    string   `json:"admin"`
	Network  *Network `json:"networks"`
	Ports    []*Ports `json:"ports"`
	Class    []*Class `json:"classes"`
	Operator []*Oper  `json:"operators"`
}

func (c *Configuration) Check() (okay bool) {
	okay = true

	// Check hostname: require at least one .
	if !ValidServerName(c.Name) {
		Error.Printf("invalid server name %q: must match /\\w+(.\\w+)+/", c.Name)
		okay = false
	}

	// Check prefix; [num][alphanum][alphanum]
	if !ValidServerPrefix(c.SID) {
		Error.Printf("invalid server [refix %q: must match /[0-9][0-9A-Z]{2}/")
		okay = false
	}
	UserIDPrefix = c.SID

	// Check opers
	if len(c.Operator) == 0 {
		Error.Printf("no operators defined: at least one required")
		okay = false
	}

	return
}

// A suitable default configuration which an admin should
// base his ircd.conf.
var DefaultConfiguration = Configuration{
	Name: "blight.local",
	SID:  "8LI",
	Admin: "Foo Bar [foo@bar.com]",
	Network: &Network{
		Name:        "IRCD-Blight",
		Description: "An unconfigured IRC network.",
		Link: []*Link{
			&Link{
				Name: "blight2.local",
				Host: []string{
					"blight2.localdomain.local",
					"127.0.0.1",
				},
				Flag: []string{
					"leaf",
				},
			},
		},
	},
	Ports: []*Ports{
		&Ports{
			PortString: "6666-6669",
		},
		&Ports{
			PortString: "6696-6699,9999",
			SSL:        true,
		},
	},
	Class: []*Class{
		&Class{
			Name: "users",
			Host: []string{
				"*",
			},
			Flag: []string{
				"noident",
			},
		},
	},
	Operator: []*Oper{
		&Oper{
			Name: "god",
			Password: &Password{
				Type:     "plain",
				Password: "blight",
			},
			Host: []string{
				"127.0.0.1",
				"*.google.com",
			},
			Flag: []string{
				"admin",
				"oper",
			},
		},
	},
}

var Config *Configuration

// LoadConfigFile loads an JSON configuration string
// as the configuration for the server.
func LoadConfigString(c string) error {
	conf, err := parseConfig([]byte(c))
	if err != nil {
		return err
	}
	Config = conf
	return nil
}

// LoadConfigFile loads an JSON configuration file
// as the configuration for the server.
func LoadConfigFile(filename string) error {
	c, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	conf, err := parseConfig([]byte(c))
	if err != nil {
		return err
	}
	Config = conf
	return nil
}

func parseConfig(c []byte) (conf *Configuration, err error) {
	conf = &Configuration{}
	err = json.Unmarshal(c, conf)
	return
}

func CheckConfig() bool {
	if Config == nil {
		Error.Printf("No configuration loaded")
		return false
	}

	return Config.Check()
}
