
Access to individual cipool's is restricted by tokens
## Token administration
Tokens are held in the "ofcir-tokens" secret and can be manipulated with the ofcirtokens.sh helper script. Each token as assosiated with a list of pools that it allows access to.
**Listing Tokens**:

    $ ./ofcirtokens.sh list
    7d934a66-f44a-4b49-97ac-0f26d05220f3 *
    adcddbec-9a83-43cb-bf44-afae3d673cf6 smallhosts
In the above example the first token allows access to all pools, while users using the second token will only have access to cir's in the pool "cipool-smallhosts"

**Creating new tokens**
New tokens can be created with a list of pools

    $ ./ofcirtokens.sh new -p smallshosts,mediumhosts
    secret/ofcir-tokens patched

or by specifying another token from which to copy pools

    $ ./ofcirtokens.sh new -t 4412af84-6400-4330-a054-f041d3adb211
    secret/ofcir-tokens patched

**Deleting tokens**

    $ ./ofcirtokens.sh delete 8f1250b9-44b7-46d5-95d3-df0270cdbc6b
    secret/ofcir-tokens patched

**Updating tokens**
tokens can be updated to increase/reduce the scope of the token

    $ ./ofcirtokens.sh list | grep 4412af84-6400-4330-a054-f041d3adb211
    4412af84-6400-4330-a054-f041d3adb211 smallshosts,mediumhosts
    $ ./ofcirtokens.sh update 4412af84-6400-4330-a054-f041d3adb211 smallshosts,mediumhosts,largehosts
    secret/ofcir-tokens patched
    $ ./ofcirtokens.sh list | grep 4412af84-6400-4330-a054-f041d3adb211
    4412af84-6400-4330-a054-f041d3adb211 smallshosts,mediumhosts,largehosts


## Using Tokens
When using the http API the user must include a token to use in the "X-OFCIRTOKEN" http header. The ofcirctl.sh helper script reads the value to the "$TOKEN" environment variable and includes it in any http calls to the API it makes.
