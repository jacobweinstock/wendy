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
	}
}
