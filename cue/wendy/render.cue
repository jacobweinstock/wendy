package wendy

import (
	"encoding/base64"
	"encoding/yaml"
	"list"
	"strings"
)

// #Runtime is what only the host can tell: addresses, names and build
// outputs. wendy fills it in Go and renders the manifests below from it, so
// every template lives here rather than in Go.
#Runtime: {
	vips: {tinkerbell: string, hookos: string, controlPlane: string}
	// Prefix length served to machines, and the IPv4 netmask it implies.
	prefixLen: int
	netmask?:  string
	subnet6?:  string
	podCIDR:     string
	serviceCIDR: string
	nameServers: [...string]
	trustedProxies: [...string]
	machines: [...#Machine]
	bmc: {host: string, user: string, pass: string}
	sshKey:     string
	agentImage: string
	images: {oci2disk: string, writefile: string, kexec: string, waitdaemon: string, alpine: string}
	osRegistry: string
	// Registries CaptainOS pulls from without verifying TLS, by address: the
	// local registry and the mirrors.
	insecureRegistries: [...string]
	// Set when Tinkerbell is built from source.
	tinkerbellImage?: string
	capt: {version: string, repository: string, imageRepository?: string}
	// The files containerd reads for a registry, keyed by registry host and
	// then file name.
	registryFiles: [string]: [string]: string
	// Host directory mounted into kind nodes at /etc/containerd/certs.d.
	certsDir?: string
	// Set when the testbed pulls through the mirrors, which serve TLS for
	// their container names.
	mirror?: {
		// The PEM CA the mirrors' certificates chain to.
		ca: string
		// /etc/hosts lines giving each mirror's address on this testbed's
		// network.
		hosts: string
	}
	dns64IP?:  string
	nat64Prefix?: string
	bridge?:   string
}

#Machine: {
	name:    string
	role:    string
	mac:     string
	ip:      string
	gateway: string
	arch:    string
	bmcPort: int
	labels: [string]: string
}

#Render: {
	// A concrete #ResolvedTestbed, as wendy decoded it. It is not re-unified
	// with the schema: its defaults are already resolved.
	testbed: {...}
	rt:      #Runtime
	// The workload cluster's Kubernetes version. An upgrade renders with a
	// new one.
	kubernetesVersion: *testbed.workloadCluster.kubernetesVersion | string

	_v6:       testbed.network.family == "ipv6"
	_ns:       testbed.tinkerbell.namespace
	_name:     testbed.workloadCluster.name
	_boot:     testbed.tinkerbell.bootMode
	_dhcpArch: {amd64: "x86_64", arm64: "aarch64"}[testbed.arch]

	// IPv6 literals are bracketed wherever they sit in a URL or host:port.
	#host: {
		addr: string
		out:  [if _v6 {"[\(addr)]"}, addr][0]
	}
	_tinkHost: (#host & {addr: rt.vips.tinkerbell}).out
	_hookHost: (#host & {addr: rt.vips.hookos}).out

	kind: {
		kind:       "Cluster"
		apiVersion: "kind.x-k8s.io/v1alpha4"
		// An ipv6 cluster keeps IPv4 on its node containers, so image pulls
		// still work while every Kubernetes address is IPv6.
		if _v6 {
			networking: ipFamily: "ipv6"
		}
		if rt.certsDir != _|_ {
			nodes: [{
				role: "control-plane"
				// kind's containerd already reads /etc/containerd/certs.d, and
				// containerd v2 rejects inline mirrors alongside config_path.
				extraMounts: [{hostPath: rt.certsDir, containerPath: "/etc/containerd/certs.d", readOnly: true}]
			}]
		}
	}

	_flavor: testbed.tinkerbell.captainos.flavor
	tinkerbellValues: {
		trustedProxies: rt.trustedProxies
		deployment: {
			// Smee bakes this into the iPXE cmdline, and the OSIE that pulls
			// it has no certs.d of its own, so its host has to be in the string.
			agentImage: rt.agentImage
			if rt.tinkerbellImage != _|_ {
				image: rt.tinkerbellImage
			}
			envs: {
				globals: backendKubeRenderingEnabled: true
				ui: enableAutoLogin:                  true
				if _v6 {
					// Every listener is IPv4-only until this says otherwise.
					globals: listenerFamilies: "ipv6"
				}
				smee: {
					if len(rt.insecureRegistries) > 0 {
						ipxeHttpScriptExtraKernelArgs: ["insecure_registries=" + strings.Join(rt.insecureRegistries, ",")]
					}
					if rt.mirror != _|_ {
						// Serves the Hardware's osieFiles, which carry the
						// mirrors' names and CA to CaptainOS.
						bootstrapEnabled: true
					}
					if _v6 {
						dhcpv6Enabled:                  true
						dhcpv6Mode:                     "derived"
						dhcpv6DerivedDirectAddressPool: rt.subnet6
						dhcpv6DefaultNameServers:       rt.nameServers
						// Without an interface Smee joins no multicast group and
						// never hears a solicit.
						dhcpv6BindInterface: "macvlan0"
					}
					if _boot == "isoboot" {
						dhcpEnabled:    false
						isoUpstreamURL: "http://\(_hookHost):7173/captainos-\(_flavor)-\(_dhcpArch).iso"
					}
					if _boot == "isoboot" && _v6 {
						// Static IPAM patching is IPv4-only and answers /iso6/ with 400.
						isoStaticIPAMEnabled: false
					}
				}
			}
		}
		if !_v6 {
			publicIP:            rt.vips.tinkerbell
			artifactsFileServer: "http://\(_hookHost):7173"
		}
		if _v6 {
			publicIPv6:            rt.vips.tinkerbell
			artifactsFileServerV6: "http://\(_hookHost):7173"
		}
		optional: captainos: {
			// Smee names the kernel and initrd after the flavor, not the image.
			flavor: _flavor
			image:  testbed.tinkerbell.captainos.image
		}
	}

	hardware: [for m in rt.machines {
		apiVersion: "tinkerbell.org/v1alpha1"
		kind:       "Hardware"
		metadata: {
			name:      m.name
			namespace: _ns
			labels: m.labels & {
				"tinkerbell.org/role":        m.role
				"node.cluster.x-k8s.io/rack": "test-\(m.role)"
			}
		}
		spec: {
			bmcRef: {apiGroup: "bmc.tinkerbell.org", kind: "Machine", name: m.name}
			disks: [{device: "/dev/vda"}]
			interfaces: [{
				dhcp: {
					arch:     {amd64: "x86_64", arm64: "aarch64"}[m.arch]
					hostname: m.name
					ip: {
						address: m.ip
						gateway: m.gateway
						// Smee rejects a netmask alongside an IPv6 address.
						if !_v6 {
							netmask: rt.netmask
						}
					}
					// Reservations make the lease time moot; the maximum keeps
					// clients from renewing constantly.
					lease_time:   4294967294
					mac:          m.mac
					uefi:         true
					name_servers: rt.nameServers
				}
				netboot: {allowPXE: true, allowWorkflow: true}
			}]
			metadata: {
				instance: {hostname: m.name, id: m.mac}
				if rt.mirror != _|_ {
					// CaptainOS sees these under /osie_files.
					osieFiles: [
						{path: "wendy/hosts", contents: rt.mirror.hosts, mode: "0644"},
						{path: "wendy/mirror-ca.crt", contents: rt.mirror.ca, mode: "0644"},
					]
				}
			}
		}
	}]

	// bmclib builds `host:port` without bracketing IPv6, so it is done here.
	_bmcHost: (#host & {addr: rt.bmc.host}).out
	_preferred: {redfish: ["gofish", "ipmitool"], ipmi: ["ipmitool", "gofish"]}[testbed.hardware.bmc.protocol]
	bmcMachines: [for m in rt.machines {
		apiVersion: "bmc.tinkerbell.org/v1alpha1"
		kind:       "Machine"
		metadata: {name: m.name, namespace: _ns}
		spec: connection: {
			authSecretRef: {name: "bmc-creds", namespace: _ns}
			host:        _bmcHost
			insecureTLS: true
			port:        [if testbed.hardware.bmc.protocol == "redfish" {443}, m.bmcPort][0]
			providerOptions: {
				preferredOrder: _preferred
				ipmitool: port: m.bmcPort
				redfish: {useBasicAuth: true, systemName: m.name}
			}
		}
	}]

	bmcSecret: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      "bmc-creds"
			namespace: _ns
			labels: {"clusterctl.cluster.x-k8s.io/move": "", "clusterctl.cluster.x-k8s.io": ""}
		}
		type: "kubernetes.io/basic-auth"
		data: {
			username: base64.Encode(null, rt.bmc.user)
			password: base64.Encode(null, rt.bmc.pass)
		}
	}

	clusterctl: {
		providers: [{
			name: "tinkerbell"
			url:  "\(rt.capt.repository)/\(rt.capt.version)/infrastructure-components.yaml"
			type: "InfrastructureProvider"
		}]
		images: "infrastructure-tinkerbell": {
			tag: rt.capt.version
			if rt.capt.imageRepository != _|_ {
				repository: rt.capt.imageRepository
			}
		}
		TINKERBELL_IP: rt.vips.tinkerbell
	}

	// What containerd needs to pull from the local registry and the mirrors.
	_hostsFiles: [for host, files in rt.registryFiles for name, body in files {
		path:        "/etc/containerd/certs.d/\(host)/\(name)"
		owner:       "root:root"
		permissions: "0644"
		content:     body
	}]
	// Ubuntu's containerd does not set config_path; kind's does.
	_configPath: [if len(_hostsFiles) > 0 {{
		path:        "/etc/containerd/conf.d/registry-config-path.toml"
		owner:       "root:root"
		permissions: "0644"
		content: """
			version = 2
			[plugins."io.containerd.grpc.v1.cri".registry]
			  config_path = "/etc/containerd/certs.d"
			"""
	}}]
	// The mirrors are reached by the names their certificates are for, which
	// only this testbed's network gives addresses to.
	_mirrorHostsFile: [if rt.mirror != _|_ {{
		path:        "/etc/wendy/mirror-hosts"
		owner:       "root:root"
		permissions: "0644"
		content:     rt.mirror.hosts
	}}]
	_mirrorPreCmds: list.Concat([
		[if rt.mirror != _|_ {"cat /etc/wendy/mirror-hosts >> /etc/hosts"}],
		[if len(_hostsFiles) > 0 {"systemctl restart containerd"}],
	])

	// kubeadm's preflight needs IPv6 forwarding, which the image does not set.
	_ipv6Files: [if _v6 {{
		path:        "/etc/sysctl.d/99-k8s-ipv6-forwarding.conf"
		owner:       "root:root"
		permissions: "0644"
		content: """
			net.ipv6.conf.all.forwarding=1
			net.ipv6.conf.default.forwarding=1
			"""
	}}]
	_ipv6PreCmds: [if _v6 {"sysctl --system"}]
	// kube-vip adds the VIP to the default-route interface, and on IPv6 it can
	// win kubelet's address selection, so the node IP is pinned first.
	_nodeIPCmds: [if _v6 {#"mkdir -p /etc/default && printf 'KUBELET_EXTRA_ARGS=--node-ip=%s\n' "$(ip -6 -j addr show dev $(ip -6 -j route list default | jq -r .[0].dev) scope global | jq -r '.[0].addr_info[0].local')" > /etc/default/kubelet"#}]

	_files:   list.Concat([_hostsFiles, _configPath, _mirrorHostsFile, _ipv6Files])
	_preCmds: list.Concat([_mirrorPreCmds, _ipv6PreCmds])

	_dashed:   strings.Replace(kubernetesVersion, ".", "-", -1)
	_kcpName:  "\(_name)-control-plane"
	_mdName:   "\(_name)-worker-a"
	_tmtCP:    "\(_name)-control-plane-\(_dashed)"
	_tmtWorker: "\(_name)-worker-a-\(_dashed)"
	_users: [{name: "tink", sudo: "ALL=(ALL) NOPASSWD:ALL", sshAuthorizedKeys: [rt.sshKey]}]
	_osVersion: strings.Replace(testbed.workloadCluster.osVersion, ".", "", -1)
	_imgURL:    "\(rt.osRegistry):\(_osVersion)-\(kubernetesVersion)-\(testbed.arch).gz"
	_kubevip:   testbed.workloadCluster.kubeVipVersion
	_routeDev:  [if _v6 {"$(ip -6 -j route list default | jq -r .[0].dev)"}, "$(ip -4 -j route list default | jq -r .[0].dev)"][0]
	_kubeVipCmd: strings.Join([
		"if [ $(cat /etc/kubernetes-version | awk -F. '{print $2}') -ge 29 ] && [ -f /run/kubeadm/kubeadm.yaml ]; then export KUBE_FILE=/etc/kubernetes/super-admin.conf; else export KUBE_FILE=/etc/kubernetes/admin.conf; fi",
		"mkdir -p /etc/kubernetes/manifests",
		"ctr images pull ghcr.io/kube-vip/kube-vip:v\(_kubevip)",
		"ctr run --rm --net-host ghcr.io/kube-vip/kube-vip:v\(_kubevip) vip /kube-vip manifest pod --arp --interface \(_routeDev) --address \(rt.vips.controlPlane) --controlplane --leaderElection --k8sConfigPath $KUBE_FILE > /etc/kubernetes/manifests/kube-vip.yaml",
	], " && ")
	_providerID: [{name: "provider-id", value: "PROVIDER_ID"}]
	_poolLabels: {"cluster.x-k8s.io/cluster-name": _name, pool: "worker-a"}

	// Object names, so steps can find what they rendered.
	names: {
		cluster:           _name
		controlPlane:      _kcpName
		machineDeployment: _mdName
		templateCP:        _tmtCP
		templateWorker:    _tmtWorker
	}

	workloadCluster: [
		{
			apiVersion: "cluster.x-k8s.io/v1beta2"
			kind:       "Cluster"
			metadata: {name: _name, namespace: _ns}
			spec: {
				clusterNetwork: {
					pods: cidrBlocks: [rt.podCIDR]
					services: cidrBlocks: [rt.serviceCIDR]
				}
				controlPlaneEndpoint: {host: rt.vips.controlPlane, port: 6443}
				controlPlaneRef: {apiGroup: "controlplane.cluster.x-k8s.io", kind: "KubeadmControlPlane", name: _kcpName}
				infrastructureRef: {apiGroup: "infrastructure.cluster.x-k8s.io", kind: "TinkerbellCluster", name: _name}
			}
		},
		{
			apiVersion: "infrastructure.cluster.x-k8s.io/v1beta1"
			kind:       "TinkerbellCluster"
			metadata: {name: _name, namespace: _ns}
			spec: imageLookupBaseRegistry: ""
		},
		{
			apiVersion: "controlplane.cluster.x-k8s.io/v1beta2"
			kind:       "KubeadmControlPlane"
			metadata: {name: _kcpName, namespace: _ns}
			spec: {
				replicas: testbed.hardware.machines.controlPlane.count
				version:  kubernetesVersion
				machineTemplate: spec: infrastructureRef: {apiGroup: "infrastructure.cluster.x-k8s.io", kind: "TinkerbellMachineTemplate", name: _tmtCP}
				kubeadmConfigSpec: {
					initConfiguration: nodeRegistration: kubeletExtraArgs: _providerID
					joinConfiguration: nodeRegistration: {
						ignorePreflightErrors: ["DirAvailable--etc-kubernetes-manifests"]
						kubeletExtraArgs: _providerID
					}
					preKubeadmCommands: list.Concat([_preCmds, _nodeIPCmds, [_kubeVipCmd]])
					if len(_files) > 0 {
						files: _files
					}
					users: _users
				}
			}
		},
		{
			apiVersion: "cluster.x-k8s.io/v1beta2"
			kind:       "MachineDeployment"
			metadata: {name: _mdName, namespace: _ns, labels: _poolLabels}
			spec: {
				clusterName: _name
				replicas:    testbed.hardware.machines.worker.count
				selector: matchLabels: _poolLabels
				template: {
					metadata: labels: _poolLabels
					spec: {
						clusterName: _name
						version:     kubernetesVersion
						bootstrap: configRef: {apiGroup: "bootstrap.cluster.x-k8s.io", kind: "KubeadmConfigTemplate", name: _mdName}
						infrastructureRef: {apiGroup: "infrastructure.cluster.x-k8s.io", kind: "TinkerbellMachineTemplate", name: _tmtWorker}
					}
				}
			}
		},
		{
			apiVersion: "bootstrap.cluster.x-k8s.io/v1beta2"
			kind:       "KubeadmConfigTemplate"
			metadata: {name: _mdName, namespace: _ns}
			spec: template: spec: {
				joinConfiguration: nodeRegistration: kubeletExtraArgs: _providerID
				if len(_files) > 0 {
					files: _files
				}
				if len(_preCmds) > 0 {
					preKubeadmCommands: _preCmds
				}
				users: _users
			}
		},
		(#tmt & {_n: _tmtCP, _role: "control-plane"}).out,
		(#tmt & {_n: _tmtWorker, _role: "worker"}).out,
	]

	#tmt: {
		_n:    string
		_role: string
		out: {
			apiVersion: "infrastructure.cluster.x-k8s.io/v1beta1"
			kind:       "TinkerbellMachineTemplate"
			metadata: {name: _n, namespace: _ns}
			spec: template: spec: {
				bootOptions: _bootOptions
				hardwareAffinity: required: [{labelSelector: matchLabels: "tinkerbell.org/role": _role}]
				templateOverride: yaml.Marshal(_workflow)
			}
		}
	}

	_bootOptions: {
		bootMode: _boot
		if _boot == "isoboot" {
			// CAPT inserts the MAC before the file name. /iso6/ makes Smee
			// patch in its IPv6 endpoints.
			isoURL: "http://\(_tinkHost):7080/\([if _v6 {"iso6"}, "iso"][0])/hook.iso"
		}
	}

	_partition3: "{{ formatPartition ( index .Hardware.Disks 0 ) 3 }}"
	#writefile: {
		_name:     string
		_path:     string
		_contents: string
		_mode:     *"0600" | string
		_dirMode:  *"0700" | string
		out: {
			name:    _name
			image:   rt.images.writefile
			timeout: 90
			environment: {
				DEST_DISK: _partition3
				FS_TYPE:   "ext4"
				DEST_PATH: _path
				UID:       "0"
				GID:       "0"
				MODE:      _mode
				DIRMODE:   _dirMode
				CONTENTS:  _contents
			}
		}
	}

	_netplanRoute: [if _v6 {"\"::/0\""}, "default"][0]
	_netplanV6:    [if _v6 {"      dhcp4: false\n      dhcp6: false\n      accept-ra: false\n"}, ""][0]
	_netplan: """
		network:
		  version: 2
		  renderer: networkd
		  ethernets:
		    id0:
		      match:
		        macaddress: {{ (index .Hardware.Interfaces 0).DHCP.MAC }}
		\(_netplanV6)      addresses:
		        - {{ (index .Hardware.Interfaces 0).DHCP.IP.Address }}/\(rt.prefixLen)
		      nameservers:
		        addresses: [{{ (index .Hardware.Interfaces 0).DHCP.NameServers | join "," }}]
		      routes:
		        - to: \(_netplanRoute)
		          via: {{ (index .Hardware.Interfaces 0).DHCP.IP.Gateway }}
		      optional: true

		"""

	// isoboot has no DHCP after boot, and cloud-init's fallback brings up
	// DHCPv4 only, so both get a static netplan.
	_staticNet: [if _boot == "isoboot" || _v6 for a in [
		(#writefile & {_name: "disable cloud-init networking", _path: "/etc/cloud/cloud.cfg.d/99-disable-network-config.cfg", _contents: "network: {config: disabled}"}).out,
		(#writefile & {_name: "create static netplan", _path: "/etc/netplan/config.yaml", _contents: _netplan, _dirMode: "0755"}).out,
	] {a}]

	// tink-agent gives a host-network action a copy of CaptainOS's /etc/hosts,
	// so the mirrors' names must be there before the image is streamed by name.
	_mirrorHostsAction: [if rt.mirror != _|_ {{
		name:    "add mirror hosts"
		image:   rt.images.alpine
		timeout: 60
		command: ["sh", "-c", "cat /osie_files/wendy/hosts >> /host/etc/hosts"]
		volumes: ["/etc/hosts:/host/etc/hosts", "/osie_files/wendy:/osie_files/wendy:ro"]
	}}]

	_workflow: {
		name:           "wendy"
		global_timeout: 6000
		tasks: [{
			name:   "wendy"
			worker: "{{.device_1}}"
			volumes: ["/dev:/dev"]
			actions: list.Concat([_mirrorHostsAction, [
				{
					name:    "stream image"
					image:   rt.images.oci2disk
					timeout: 1200
					// The only action pulling over the network. tink-agent's
					// isolated netns is IPv4-only, so it shares the host's.
					namespaces: network: "host"
					if rt.mirror != _|_ {
						// Go trusts every file in /etc/ssl/certs.
						volumes: ["/osie_files/wendy/mirror-ca.crt:/etc/ssl/certs/wendy-mirror-ca.crt:ro"]
					}
					environment: {
						IMG_URL:    _imgURL
						DEST_DISK:  "{{ index .Hardware.Disks 0 }}"
						COMPRESSED: true
					}
				},
				(#writefile & {_name: "add tink cloud-init config", _path: "/etc/cloud/cloud.cfg.d/10_tinkerbell.cfg", _contents: """
					datasource:
					  Ec2:
					    metadata_urls: ["http://\(_tinkHost):7080"]
					    strict_id: false
					system_info:
					  default_user:
					    name: tink
					    groups: [wheel, adm]
					    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
					    shell: /bin/bash
					manage_etc_hosts: localhost
					warnings:
					  dsid_missing_source: off

					"""}).out,
			], _staticNet, [
				(#writefile & {_name: "add tink cloud-init ds-config", _path: "/etc/cloud/ds-identify.cfg", _contents: "datasource: Ec2\n"}).out,
				{
					name:    "kexec image"
					image:   rt.images.waitdaemon
					timeout: 90
					pid:     "host"
					environment: {
						BLOCK_DEVICE: "{{ formatPartition ( index .Hardware.Disks 0 ) 1 }}"
						FS_TYPE:      "vfat"
						IMAGE:        rt.images.kexec
						GRUBCFG_PATH: "/grub/grub.cfg"
						WAIT_SECONDS: 10
					}
					volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
				},
			]])
		}]
	}

	machineHealthCheck: {
		apiVersion: "cluster.x-k8s.io/v1beta2"
		kind:       "MachineHealthCheck"
		metadata: {name: "\(_mdName)-unhealthy", namespace: _ns}
		spec: {
			clusterName: _name
			selector: matchLabels: _poolLabels
			checks: {
				nodeStartupTimeoutSeconds: 1200
				unhealthyNodeConditions: [
					{type: "Ready", status: "Unknown", timeoutSeconds: 60},
					{type: "Ready", status: "False", timeoutSeconds: 60},
				]
			}
		}
	}

	// Router Advertisements. Tinkerbell sends none, and without one networkd
	// never starts its DHCPv6 client. A=0, L=0 keeps Smee's macvlan0 from
	// taking an address or route that would pull pod traffic onto it.
	radvd: """
		interface \(rt.bridge)
		{
		    AdvSendAdvert on;
		    AdvManagedFlag on;
		    AdvOtherConfigFlag on;
		    MinRtrAdvInterval 3;
		    MaxRtrAdvInterval 10;
		    prefix \(rt.subnet6)
		    {
		        AdvOnLink off;
		        AdvAutonomous off;
		        AdvRouterAddr on;
		    };
		};

		"""

	// translate_all: a dual-stack upstream would otherwise be answered with
	// its real IPv6 address, which a host without an IPv6 uplink cannot reach.
	dns64Corefile: """
		.:53 {
		    errors
		    dns64 {
		        prefix \(rt.nat64Prefix)
		        translate_all
		    }
		    forward . 1.1.1.1 8.8.8.8
		    cache 30
		}

		"""
}
