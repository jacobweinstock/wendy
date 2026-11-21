// Written by wendy's UI. Edit entries in the UI, not here: the UI rewrites this file.
// Put hand-written config in its own file.
package local

import "github.com/tinkerbell/wendy/cue/wendy"

wendy.#Config & {
	parts: {
		hardware: {
			"1x1": {
				machines: {
					spare: {
						count: 0
					}
				}
			}
			"1x1-arm64": {
				arch: "arm64"
				machines: {
					spare: {
						count: 0
					}
				}
			}
			medium: {
				machines: {
					controlPlane: {
						count: 2
					}
					spare: {
						count: 2
					}
					worker: {
						count: 2
					}
				}
			}
		}
		tinkerbell: {
			local: {
				source: {
					repo: "/home/tink/repos/tinkerbell/tinkerbell"
				}
			}
			mcanevet: {
				source: {
					ref:  "fix/tink-controller-workflow-requeue"
					repo: "https://github.com/mcanevet/tinkerbell"
				}
			}
		}
	}
	plans: {
		"quickstart-ipv6": {
			keep: "always"
			suites: ["smoke"]
			testbed: "quickstart-ipv6"
		}
	}
	testbeds: {
		"capt-e2e-copy": {
			description: "The CAPT playground's e2e matrix: topology × IP family × boot mode × registry"
			hardware:    "small-no-spare"
			matrix: {
				axes: {
					bootMode: {
						isoboot: {
							bootMode: "isoboot"
						}
						netboot: {
							bootMode: "netboot"
						}
					}
					ipFamily: {
						ipv4: {
							network: {
								family: "ipv4"
							}
						}
						ipv6: {
							network: {
								family: "ipv6"
							}
						}
					}
					registry: {
						direct: {
							network: {
								mirror: {
									enabled: false
								}
							}
						}
						mirror: {
							network: {
								mirror: {
									enabled: true
								}
							}
						}
					}
					topology: {
						colocated: {
							topology: "colocated"
						}
						external: {
							topology: "external"
						}
					}
				}
				exclude: []
				order: ["topology", "ipFamily", "bootMode", "registry"]
			}
			tinkerbell:      "mcanevet"
			workloadCluster: "k8s-1.35-kube-router"
		}
		"quickstart-arm64": {
			hardware: "1x1-arm64"
		}
		"quickstart-ipv6": {
			bootMode: "isoboot"
			hardware: "1x1"
			network: {
				family: "ipv6"
			}
			workloadCluster: "k8s-1.36-kube-router"
		}
	}
}
