package config

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

// Transformer is a function which takes configuration and applies some filter to it
type Transformer func(c *Config) error

// Profile contains the profile transformer the description of the profile
type Profile struct {
	// Description briefly describes the functionality of the profile
	Description string

	// Transform takes ipfs configuration and applies the profile to it
	Transform Transformer
}

// defaultServerFilters has is a list of IPv4 and IPv6 prefixes that are private, local only, or unrouteable.
// according to https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml
// and https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml
var defaultServerFilters = []string{
	"/ip4/10.0.0.0/ipcidr/8",
	"/ip4/100.64.0.0/ipcidr/10",
	"/ip4/169.254.0.0/ipcidr/16",
	"/ip4/172.16.0.0/ipcidr/12",
	"/ip4/192.0.0.0/ipcidr/24",
	"/ip4/192.0.0.0/ipcidr/29",
	"/ip4/192.0.0.8/ipcidr/32",
	"/ip4/192.0.0.170/ipcidr/32",
	"/ip4/192.0.0.171/ipcidr/32",
	"/ip4/192.0.2.0/ipcidr/24",
	"/ip4/192.168.0.0/ipcidr/16",
	"/ip4/198.18.0.0/ipcidr/15",
	"/ip4/198.51.100.0/ipcidr/24",
	"/ip4/203.0.113.0/ipcidr/24",
	"/ip4/240.0.0.0/ipcidr/4",
	"/ip6/100::/ipcidr/64",
	"/ip6/2001:2::/ipcidr/48",
	"/ip6/2001:db8::/ipcidr/32",
	"/ip6/fc00::/ipcidr/7",
	"/ip6/fe80::/ipcidr/10",
}

func ExternalIP() (string, error) {
	resp, err := http.Get("http://checkip.amazonaws.com")
	if err != nil {
		fmt.Println("get external IP failed")
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("parse external IP failed")
		return "", err
	}
	ip := string(body)
	// remove last return
	ip = ip[:len(ip)-1]
	address := "/ip4/" + ip
	address += "/tcp/4001"
	return address, nil
}

// Profiles is a map holding configuration transformers. Docs are in docs/config.md
var Profiles = map[string]Profile{
	"server": {
		Description: `Disables local host discovery, recommended when
running IPFS on machines with public IPv4 addresses.`,

		Transform: func(c *Config) error {
			c.Addresses.NoAnnounce = appendSingle(c.Addresses.NoAnnounce, defaultServerFilters)
			c.Swarm.AddrFilters = appendSingle(c.Swarm.AddrFilters, defaultServerFilters)
			c.Discovery.MDNS.Enabled = false
			c.Swarm.DisableNatPortMap = true
			return nil
		},
	},

	"local-discovery": {
		Description: `Sets default values to fields affected by the server
profile, enables discovery in local networks.`,

		Transform: func(c *Config) error {
			c.Addresses.NoAnnounce = deleteEntries(c.Addresses.NoAnnounce, defaultServerFilters)
			c.Swarm.AddrFilters = deleteEntries(c.Swarm.AddrFilters, defaultServerFilters)
			c.Discovery.MDNS.Enabled = true
			c.Swarm.DisableNatPortMap = false
			return nil
		},
	},
	"test": {
		Description: `Reduces external interference of IPFS daemon, this
is useful when using the daemon in test environments.`,

		Transform: func(c *Config) error {
			c.Addresses.API = Strings{"/ip4/127.0.0.1/tcp/0"}
			c.Addresses.Gateway = Strings{"/ip4/127.0.0.1/tcp/0"}
			c.Addresses.Swarm = []string{
				"/ip4/127.0.0.1/tcp/0",
			}

			c.Swarm.DisableNatPortMap = true

			c.Bootstrap = []string{}
			c.Discovery.MDNS.Enabled = false
			return nil
		},
	},
	"default-networking": {
		Description: `Restores default network settings.
Inverse profile of the test profile.`,

		Transform: func(c *Config) error {
			c.Addresses = addressesConfig()

			bootstrapPeers, err := DefaultBootstrapPeers()
			if err != nil {
				return err
			}
			c.Bootstrap = appendSingle(c.Bootstrap, BootstrapPeerStrings(bootstrapPeers))

			c.Swarm.DisableNatPortMap = false
			c.Discovery.MDNS.Enabled = true
			return nil
		},
	},
	"announce-public": {
		Description: `Announce public IP when running on cloud VM or local network.`,
		Transform: func(c *Config) error {
			address, err := ExternalIP()
			if err != nil {
				return err
			}
			c.Addresses.Announce = appendSingle(c.Addresses.Announce, []string{address})
			return nil
		},
	},
	"badgerds": {
		Description: `Replaces default datastore configuration with experimental
badger datastore.

If you apply this profile after ipfs init, you will need
to convert your datastore to the new configuration.
You can do this using ipfs-ds-convert.

For more on ipfs-ds-convert see
$ ipfs-ds-convert --help
and
$ ipfs-ds-convert convert --help

WARNING: badger datastore is experimental.
Make sure to backup your data frequently.`,

		Transform: func(c *Config) error {
			c.Datastore.Spec = map[string]interface{}{
				"type":   "measure",
				"prefix": "badger.datastore",
				"child": map[string]interface{}{
					"type":       "badgerds",
					"path":       "badgerds",
					"syncWrites": true,
					"truncate":   true,
				},
			}
			return nil
		},
	},
	"default-datastore": {
		Description: `Restores default datastore configuration.

If you apply this profile after ipfs init, you will need
to convert your datastore to the new configuration.
You can do this using ipfs-ds-convert.

For more on ipfs-ds-convert see
$ ipfs-ds-convert --help
and
$ ipfs-ds-convert convert --help
`,

		Transform: func(c *Config) error {
			c.Datastore.Spec = DefaultDatastoreConfig().Spec
			return nil
		},
	},
	"lowpower": {
		Description: `Reduces daemon overhead on the system. May affect node
functionality - performance of content discovery and data
fetching may be degraded.
`,
		Transform: func(c *Config) error {
			c.Routing.Type = "dhtclient"
			c.Reprovider.Interval = "0"

			c.Swarm.ConnMgr.LowWater = 20
			c.Swarm.ConnMgr.HighWater = 40
			c.Swarm.ConnMgr.GracePeriod = time.Minute.String()
			return nil
		},
	},
	"randomports": {
		Description: `Use a random port number for swarm.`,

		Transform: func(c *Config) error {
			port, err := getAvailablePort()
			if err != nil {
				return err
			}
			c.Addresses.Swarm = []string{
				fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
				fmt.Sprintf("/ip6/::/tcp/%d", port),
			}
			return nil
		},
	},
	"storage-host": {
		Description: `Configures necessary flags and options for node to become a storage host.`,

		Transform: func(c *Config) error {
			bootstrapPeers, err := DefaultBootstrapPeers()
			if err != nil {
				return err
			}
			c.Bootstrap = BootstrapPeerStrings(bootstrapPeers)
			c.Experimental.Libp2pStreamMounting = true
			c.Experimental.StorageHostEnabled = true
			c.Experimental.Analytics = true
			if len(c.Addresses.RemoteAPI) == 0 {
				c.Addresses.RemoteAPI = Strings{"/ip4/0.0.0.0/tcp/5101"}
			}
			if c.Datastore.StorageMax == "10GB" {
				c.Datastore.StorageMax = "1TB"
			}
			c.Services = DefaultServicesConfig()
			c.Swarm.SwarmKey = DefaultSwarmKey
			return nil
		},
	},
	"storage-host-dev": {
		Description: `[dev] Configures necessary flags and options for node to become a storage host.`,

		Transform: func(c *Config) error {
			if err := transformDevStorageHost(c); err != nil {
				return err
			}
			c.Services = DefaultServicesConfigDev()
			return nil
		},
	},
	"storage-host-testnet": {
		Description: `[testnet] Configures necessary flags and options for node to become a storage host.`,

		Transform: func(c *Config) error {
			if err := transformDevStorageHost(c); err != nil {
				return err
			}
			c.Services = DefaultServicesConfigTestnet()
			return nil
		},
	},
	"storage-client": {
		Description: `Configures necessary flags and options for node to pay to store files on the network.`,

		Transform: func(c *Config) error {
			bootstrapPeers, err := DefaultBootstrapPeers()
			if err != nil {
				return err
			}
			c.Bootstrap = BootstrapPeerStrings(bootstrapPeers)
			c.Experimental.Libp2pStreamMounting = true
			c.Experimental.StorageClientEnabled = true
			c.Experimental.HostsSyncEnabled = true
			c.Experimental.HostsSyncMode = DefaultHostsSyncMode.String()
			if len(c.Addresses.RemoteAPI) == 0 {
				c.Addresses.RemoteAPI = Strings{"/ip4/0.0.0.0/tcp/5101"}
			}
			c.Services = DefaultServicesConfig()
			c.Swarm.SwarmKey = DefaultSwarmKey
			return nil
		},
	},
	"storage-client-dev": {
		Description: `[dev] Configures necessary flags and options for node to pay to store files on the network.`,

		Transform: func(c *Config) error {
			if err := transformDevStorageClient(c); err != nil {
				return err
			}
			c.Services = DefaultServicesConfigDev()
			return nil
		},
	},
	"storage-client-testnet": {
		Description: `[testnet] Configures necessary flags and options for node to pay to store files on the network.`,

		Transform: func(c *Config) error {
			if err := transformDevStorageClient(c); err != nil {
				return err
			}
			c.Services = DefaultServicesConfigTestnet()
			return nil
		},
	},
}

// transformDevStorageHost transforms common host settings among different dev environments
func transformDevStorageHost(c *Config) error {
	bootstrapPeers, err := DefaultTestnetBootstrapPeers()
	if err != nil {
		return err
	}
	c.Bootstrap = BootstrapPeerStrings(bootstrapPeers)
	c.Experimental.Libp2pStreamMounting = true
	c.Experimental.StorageHostEnabled = true
	c.Experimental.Analytics = true
	if len(c.Addresses.RemoteAPI) == 0 {
		c.Addresses.RemoteAPI = Strings{"/ip4/0.0.0.0/tcp/5101"}
	}
	if c.Datastore.StorageMax == "10GB" {
		c.Datastore.StorageMax = "1TB"
	}
	c.Services = DefaultServicesConfigDev()
	c.Swarm.SwarmKey = DefaultTestnetSwarmKey
	return nil
}

// transformDevStorageClient transforms common client settings among different dev environments
func transformDevStorageClient(c *Config) error {
	bootstrapPeers, err := DefaultTestnetBootstrapPeers()
	if err != nil {
		return err
	}
	c.Bootstrap = BootstrapPeerStrings(bootstrapPeers)
	c.Experimental.Libp2pStreamMounting = true
	c.Experimental.StorageClientEnabled = true
	c.Experimental.HostsSyncEnabled = true
	c.Experimental.HostsSyncMode = DefaultHostsSyncModeDev.String()
	if len(c.Addresses.RemoteAPI) == 0 {
		c.Addresses.RemoteAPI = Strings{"/ip4/0.0.0.0/tcp/5101"}
	}
	c.Swarm.SwarmKey = DefaultTestnetSwarmKey
	return nil
}

func getAvailablePort() (port int, err error) {
	ln, err := net.Listen("tcp", "[::]:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	port = ln.Addr().(*net.TCPAddr).Port
	return port, nil
}

func appendSingle(a []string, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	m := map[string]bool{}
	for _, f := range a {
		if !m[f] {
			out = append(out, f)
		}
		m[f] = true
	}
	for _, f := range b {
		if !m[f] {
			out = append(out, f)
		}
		m[f] = true
	}
	return out
}

func deleteEntries(arr []string, del []string) []string {
	m := map[string]struct{}{}
	for _, f := range arr {
		m[f] = struct{}{}
	}
	for _, f := range del {
		delete(m, f)
	}
	return mapKeys(m)
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for f := range m {
		out = append(out, f)
	}
	return out
}
