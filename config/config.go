package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coredhcp/coredhcp/logger"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

var log = logger.GetLogger()

// Config holds the DHCPv6/v4 server configuration
type Config struct {
	v       *viper.Viper
	Server6 *ServerConfig
	Server4 *ServerConfig
}

// New returns a new initialized instance of a Config object
func New() *Config {
	return &Config{v: viper.New()}
}

// ServerConfig holds a server configuration that is specific to either the
// DHCPv6 server or the DHCPv4 server.
type ServerConfig struct {
	Listener *net.UDPAddr
	Plugins  []*PluginConfig
}

// PluginConfig holds the configuration of a plugin
type PluginConfig struct {
	Name string
	Args []string
}

// Load reads a configuration file and returns a Config object, or an error if
// any.
func Load() (*Config, error) {
	log.Print("Loading configuration")
	c := New()
	c.v.SetConfigType("yml")
	c.v.SetConfigName("config")
	c.v.AddConfigPath(".")
	c.v.AddConfigPath("$HOME/.coredhcp/")
	c.v.AddConfigPath("/etc/coredhcp/")
	if err := c.v.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := c.parseConfig(true); err != nil {
		return nil, err
	}
	if err := c.parseConfig(false); err != nil {
		return nil, err
	}
	if c.Server6 == nil && c.Server4 == nil {
		return nil, ConfigErrorFromString("need at least one valid config for DHCPv6 or DHCPv4")
	}
	return c, nil
}

func parsePlugins(pluginList []interface{}) ([]*PluginConfig, error) {
	plugins := make([]*PluginConfig, 0)
	for idx, val := range pluginList {
		conf := cast.ToStringMap(val)
		if conf == nil {
			return nil, ConfigErrorFromString("dhcpv6: plugin #%d is not a string map", idx)
		}
		// make sure that only one item is specified, since it's a
		// map name -> args
		if len(conf) != 1 {
			return nil, ConfigErrorFromString("dhcpv6: exactly one plugin per item can be specified")
		}
		var (
			name string
			args []string
		)
		// only one item, as enforced above, so read just that
		for k, v := range conf {
			name = k
			args = strings.Fields(cast.ToString(v))
			break
		}
		plugins = append(plugins, &PluginConfig{Name: name, Args: args})
	}
	return plugins, nil
}

func (c *Config) getListenAddress(v6 bool) (*net.UDPAddr, error) {
	ver := 6
	if !v6 {
		ver = 4
	}
	if exists := c.v.Get(fmt.Sprintf("server%d", ver)); exists == nil {
		// it is valid to have no server configuration defined, and in this case
		// no listening address and no error are returned.
		return nil, nil
	}
	addr := c.v.GetString(fmt.Sprintf("server%d.listen", ver))
	if addr == "" {
		return nil, ConfigErrorFromString("dhcpv%v: missing `server%d.listen` directive", ver, ver)
	}
	ipStr, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, ConfigErrorFromString("dhcpv%d: %v", ver, err)
	}
	ip := net.ParseIP(ipStr)
	if v6 && ip.To4() != nil {
		return nil, ConfigErrorFromString("dhcpv%d: missing or invalid `listen` address", ver)
	} else if !v6 && ip.To4() == nil {
		return nil, ConfigErrorFromString("dhcpv%d: missing or invalid `listen` address", ver)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, ConfigErrorFromString("dhcpv%d: invalid `listen` port", ver)
	}
	listener := net.UDPAddr{
		IP:   ip,
		Port: port,
	}
	return &listener, nil
}

func (c *Config) getPlugins(v6 bool) ([]*PluginConfig, error) {
	pluginList := cast.ToSlice(c.v.Get("server6.plugins"))
	if pluginList == nil {
		return nil, ConfigErrorFromString("dhcpv6: invalid plugins section, not a list")
	}
	plugins, err := parsePlugins(pluginList)
	if err != nil {
		return nil, err
	}
	return plugins, nil
}

func (c *Config) parseConfig(v6 bool) error {
	ver := 6
	if !v6 {
		ver = 4
	}
	listenAddr, err := c.getListenAddress(v6)
	if err != nil {
		return err
	}
	if listenAddr == nil {
		// no listener is configured, so `c.Server6` (or `c.Server4` if v4)
		// will stay nil.
		return nil
	}
	// read plugin configuration
	plugins, err := c.getPlugins(v6)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		log.Printf("DHCPv%d: found plugin `%s` with %d args: %v", ver, p.Name, len(p.Args), p.Args)
	}
	sc := ServerConfig{
		Listener: listenAddr,
		Plugins:  plugins,
	}
	if v6 {
		c.Server6 = &sc
	} else {
		c.Server4 = &sc
	}
	return nil
}
