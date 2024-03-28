The ibmcloud provider can be used to make Hardware available as CIR's, the cipool must first be configured with some details to communicate with ibmcloud. In addition to this the individual nodes in ibmcloud need to have some metadata added to be used by ofcir

## ibmcloud provider secret
For ibmcloud the provider secret should be configured with the following values

**apikey**: The API key to be used to authenticate with the ibmcloud API
**preset**: The preset package name to use when creating Hourly nodes for CIR's, if not set then the provider will only be capable of using manualy precreated Hardware
**sshkey**: A public ssh key to be provisioned on the server
**os**:     The os to installed on the nodes in this pool, defaults to "OS_CENTOS_STREAM_8_X_64_BIT"

## Ibmcloud nodes
When the ibmcloud provider needs to acquire a node, it looks at the nodes "tags" in order to find a suitable node. The relevant tags are
**ofcir-manual**: The presence of this tag indicates to ofcir that a node can be managed and used to back a CIR
**ofcir-taken**: The presence of this tag indicates to ofcir that a node is already linked with an existing CIR

When trying to find Hardware to back a CIR, ofcir will search for a node with the "ofcir-manual" tag but without the "ofcir-taken" tag, it will
then add the "ofcir-taken" tag and use the node.

If no Hardware is found and "preset" is set in the config ofcir will create a Hourly node using its value as the preset package name.
