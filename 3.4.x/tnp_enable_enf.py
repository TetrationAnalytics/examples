# -*- coding: utf-8 -*-
"""

This module provides the function `tetOrchestratorEnableEnforcement` as
a convenient helper to enable/disable policy enforcement for a Tetration
external orchestrator. Note that this feature is allowed only for certain
external orchestrators. Refer to Tetration User Guide, section `External
Orchestrators` for more details.

Copyright 2020 Cisco Systems, Inc

"""

import sys, json
from tetpyclient import RestClient

def tetOrchestratorEnableEnforcement(apiEndpoint, apiCredFile, scope, orchID, username, password, enableEnf):
    """

    `tetOrchestratorEnableEnforcement` sets the configuration field `enable_enforcement` of the given
    external orchestrator that supports orchestrated policy enforcement. Once enabled via this method
    the external orchestrator will deploy the defined policies to the load balancer appliance when
    a workspace policy enforcement is performed. For more details please see Tetration User Guide,
    section `External Orchestrators`.

    Args:
        apiEndpoint (string): Tetration OpenAPI endpoint (URL)
        apiCredFile (string): Tetration API key, see User Guide, OpenAPI Authentication for details
        scope (string): Name of scope the external orchestrator belongs to
        orchID (string): External orchestrator ID
        username (string): User name used to access load balancer REST API endpoint
        password (string): Password used to access load balancer REST API endpoint
        enableEnf (bool): `True` or `False` to enable or disable policy enforcement feature

    Returns:
        None: if updating the external orchestrator was successful
        str: error string

    """

    restclient = RestClient(apiEndpoint,
                credentials_file=apiCredFile,
                verify=True)
    resp = restclient.get('/orchestrator/{}/{}'.format(scope, orchID))
    if resp.status_code != 200:
        return "ERROR: failed to get orchestrator: http_status={}, text:{}".format(resp.status_code, resp.text)

    data = resp.json()
    data["username"]=username
    data["password"]=password
    data["enable_enforcement"] = enableEnf
    resp = restclient.put('/orchestrator/{}/{}'.format(scope, orchID), json_body=json.dumps(data))
    if resp.status_code != 200:
        return "ERROR: failed to update orchestrator: http_status={}, text:{}".format(resp.status_code, resp.text)

if __name__ == "__main__":
    if len(sys.argv) != 8:
        print("Usage: {} <openapi_endpoint> <api_key_file> <scope> <orchestrator_id> <user_name> <password> enable|disable".format(sys.argv[0]))
        sys.exit(-1)

    if sys.argv[7] != "enable" and sys.argv[7] != "disable":
        print("ERROR: invalid value \"{}\"\n  Allowed values are: \"enable\", \"disable\"".format(sys.argv[7]))
        sys.exit(-1)

    enableEnf=sys.argv[7] == "enable"
    errStr=tetOrchestratorEnableEnforcement(sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6], enableEnf)
    if errStr != None:
        print(errStr)
        sys.exit(-1)

    print("updated orchestrator id={} enable_enforcement={}".format(sys.argv[4], enableEnf))
