The ironic provider can be used to make ironic nodes available as CIR's, the cipool must first be configured with some details to communicated with ironic. In addition to this the individual nodes in ironic need to have some metadata added to be used by ofcir

## ironic provider secret
For standalone ironic the provider secret should be configured with the following values

**endpoint**:   The Endpoint used to contact ironic
**username**: The username use to authenticated with the ironic APi
**password**: The password used to authenticate with the ironic API
**image**: A url to a qcow2 image used by ironic to provision nodes before making them available
**sshkey**: A public ssh key to be provisioned on the server

If using ironic with keystone auth and glance images the provider secret should be configured with the following values
**cloudyaml**: A base64 representation of a clouds.yaml file used to authenticate with openstack
**image**: The id of the image in the glance server
**sshkey**: A public ssh key to be provisioned on the server

## Ironic nodes
When the ironic provider needs to acquire a node, it looks at the nodes "extra" data in order to find a suitable node. The relevant fields in the "extra" data are
**ofcir_type**: this is set to match the "type" of the cipool, e.g. CIR's of type "large" with only be associated with ironic nodes of type "large"
**ofcir_ip**: This should be pre-populated to the IP address associated with this node, the assumption is the dhcp has been configured to allocate each node with a specific address
**ofcir_port_ssh**: This should be pre-populated to the ssh port number associated with this node, if not present port 22 can be assumed. This can be used when multiple nodes use a common Floating IP
**ofcir_cir**: This is set by the ironic provider to mark a node as already in use
**ofcir_data**: Added to the cir assosiated with this node to provised arbitary data about the node in question

To set the able data on a node the following command can be used

    baremetal node set --extra ofcir_type=large --extra ofcir_ip=<IPADDRESS> node-large-1

Ironic nodes should be either "available" or "active" before scaling an ironic pool to use them.

