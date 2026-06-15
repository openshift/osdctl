#!/bin/bash
# Helper functions for pull-secret mutating tests
# Called by test-pull-secret.expect via exec
# Usage: bash test-ps-helpers.sh <action> <args...>

set -euo pipefail

ACTION="$1"
REASON="ROSAENG-2057-mutating-test"

case "$ACTION" in
    login)
        CLUSTER_ID="$2"
        ocm backplane login "$CLUSTER_ID" 2>&1 | tail -1
        ;;

    logout)
        ocm backplane logout 2>&1 || true
        ;;

    tamper-email)
        REGISTRY="$2"
        NEW_EMAIL="$3"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        PS_JSON=$(echo "$PS_B64" | base64 -d)
        TAMPERED=$(echo "$PS_JSON" | jq --arg r "$REGISTRY" --arg e "$NEW_EMAIL" '.auths[$r].email = $e')
        NEW_B64=$(echo "$TAMPERED" | base64 | tr -d '\n')
        ocm backplane elevate "$REASON" -- patch secret pull-secret -n openshift-config \
            -p "{\"data\":{\".dockerconfigjson\":\"$NEW_B64\"}}" 2>&1
        ;;

    tamper-token)
        REGISTRY="$2"
        NEW_TOKEN="$3"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        PS_JSON=$(echo "$PS_B64" | base64 -d)
        TAMPERED=$(echo "$PS_JSON" | jq --arg r "$REGISTRY" --arg t "$NEW_TOKEN" '.auths[$r].auth = $t')
        NEW_B64=$(echo "$TAMPERED" | base64 | tr -d '\n')
        ocm backplane elevate "$REASON" -- patch secret pull-secret -n openshift-config \
            -p "{\"data\":{\".dockerconfigjson\":\"$NEW_B64\"}}" 2>&1
        ;;

    add-auth)
        REGISTRY="$2"
        TOKEN="$3"
        EMAIL="$4"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        PS_JSON=$(echo "$PS_B64" | base64 -d)
        MODIFIED=$(echo "$PS_JSON" | jq --arg r "$REGISTRY" --arg t "$TOKEN" --arg e "$EMAIL" '.auths[$r] = {auth: $t, email: $e}')
        NEW_B64=$(echo "$MODIFIED" | base64 | tr -d '\n')
        ocm backplane elevate "$REASON" -- patch secret pull-secret -n openshift-config \
            -p "{\"data\":{\".dockerconfigjson\":\"$NEW_B64\"}}" 2>&1
        ;;

    remove-auth)
        REGISTRY="$2"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        PS_JSON=$(echo "$PS_B64" | base64 -d)
        MODIFIED=$(echo "$PS_JSON" | jq --arg r "$REGISTRY" 'del(.auths[$r])')
        NEW_B64=$(echo "$MODIFIED" | base64 | tr -d '\n')
        ocm backplane elevate "$REASON" -- patch secret pull-secret -n openshift-config \
            -p "{\"data\":{\".dockerconfigjson\":\"$NEW_B64\"}}" 2>&1
        ;;

    get-email)
        REGISTRY="$2"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        echo "$PS_B64" | base64 -d | jq -r --arg r "$REGISTRY" '.auths[$r].email'
        ;;

    auth-exists)
        REGISTRY="$2"
        PS_B64=$(ocm backplane elevate "$REASON" -- get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null)
        echo "$PS_B64" | base64 -d | jq -r --arg r "$REGISTRY" '.auths | has($r)'
        ;;

    node-state)
        echo "=== Node Status ==="
        ocm backplane elevate "$REASON" -- get nodes -o wide 2>/dev/null || echo "(failed to get nodes)"
        echo ""
        echo "=== Recent Node Events (last 5m) ==="
        ocm backplane elevate "$REASON" -- get events --field-selector involvedObject.kind=Node --sort-by='.lastTimestamp' 2>/dev/null | tail -20 || echo "(failed to get events)"
        echo ""
        echo "=== Kubelet Restart Count ==="
        ocm backplane elevate "$REASON" -- get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .status.conditions[?(@.type=="Ready")]}{.lastTransitionTime}{end}{"\n"}{end}' 2>/dev/null || echo "(failed to get kubelet status)"
        echo ""
        echo "=== MachineHealthCheck ==="
        ocm backplane elevate "$REASON" -- get machinehealthcheck -A 2>/dev/null || echo "(failed to get MHC)"
        ;;

    oao-state)
        echo "=== ocm-agent-operator Pod Status ==="
        ocm backplane elevate "$REASON" -- get pods -n openshift-ocm-agent-operator -o wide 2>/dev/null || echo "(failed to get OAO pods)"
        echo ""
        echo "=== ocm-agent Pod Details (restart counts, age, state) ==="
        ocm backplane elevate "$REASON" -- get pods -n openshift-ocm-agent-operator -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\t"}{range .status.containerStatuses[*]}restarts={.restartCount} ready={.ready} {end}{"\n"}{end}' 2>/dev/null || echo "(failed to get OAO pod details)"
        echo ""
        echo "=== ocm-agent Pod Start Times ==="
        ocm backplane elevate "$REASON" -- get pods -n openshift-ocm-agent-operator -l app=ocm-agent -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.startTime}{"\n"}{end}' 2>/dev/null || echo "(failed to get start times)"
        echo ""
        echo "=== ocm-agent-operator Deployment ==="
        ocm backplane elevate "$REASON" -- get deployment -n openshift-ocm-agent-operator -o wide 2>/dev/null || echo "(failed to get OAO deployment)"
        echo ""
        echo "=== ocm-agent Deployment Generation/Revision ==="
        ocm backplane elevate "$REASON" -- get deployment ocm-agent -n openshift-ocm-agent-operator -o jsonpath='{.metadata.generation}{"\t"}{.metadata.annotations.deployment\.kubernetes\.io/revision}' 2>/dev/null || echo "(failed to get deployment revision)"
        echo ""
        echo "=== ocm-agent Access Token Secret (cloud.openshift.com email only) ==="
        ocm backplane elevate "$REASON" -- get secret -n openshift-ocm-agent-operator -l app=ocm-agent -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.resourceVersion}{"\t"}{.metadata.annotations.kubectl\.kubernetes\.io/last-applied-configuration}{"\n"}{end}' 2>/dev/null || echo "(no labeled secrets found)"
        echo ""
        echo "=== All Secrets in openshift-ocm-agent-operator (names + resourceVersion) ==="
        ocm backplane elevate "$REASON" -- get secrets -n openshift-ocm-agent-operator -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.type}{"\t"}{.metadata.resourceVersion}{"\t"}{.metadata.creationTimestamp}{"\n"}{end}' 2>/dev/null || echo "(failed to list secrets)"
        echo ""
        echo "=== ConfigMaps in openshift-ocm-agent-operator (names + resourceVersion) ==="
        ocm backplane elevate "$REASON" -- get configmaps -n openshift-ocm-agent-operator -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.resourceVersion}{"\t"}{.metadata.creationTimestamp}{"\n"}{end}' 2>/dev/null || echo "(failed to list configmaps)"
        echo ""
        echo "=== ocm-agent Events (last 10) ==="
        ocm backplane elevate "$REASON" -- get events -n openshift-ocm-agent-operator --sort-by='.lastTimestamp' 2>/dev/null | tail -10 || echo "(failed to get OAO events)"
        echo ""
        echo "=== OAO Version ==="
        ocm backplane elevate "$REASON" -- get deployment ocm-agent-operator -n openshift-ocm-agent-operator -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo "(failed to get OAO image)"
        echo ""
        echo "=== Access Token Hash (sha256 of token value, not the token itself) ==="
        ocm backplane elevate "$REASON" -- get secrets -n openshift-ocm-agent-operator -o json 2>/dev/null | \
            jq -r '.items[] | select(.data.access_token != null) | .metadata.name + "\t" + .data.access_token' 2>/dev/null | \
            while IFS=$'\t' read -r name b64token; do
                hash=$(echo "$b64token" | base64 -d 2>/dev/null | shasum -a 256 | cut -c1-12)
                echo "$name	token_hash=$hash"
            done || echo "(no access_token secret found)"
        echo ""
        ;;

    wait-for-oao-reconcile)
        # Polls the ocm-access-token Secret resourceVersion until it changes,
        # indicating OAO reconciled. Args: $2 = pre-tamper resourceVersion, $3 = timeout in seconds (default 360)
        PRE_RV="$2"
        TIMEOUT="${3:-360}"
        INTERVAL=15
        ELAPSED=0

        if [ -z "$PRE_RV" ]; then
            echo "ERROR: must provide pre-tamper resourceVersion as argument"
            echo "Usage: bash test-ps-helpers.sh wait-for-oao-reconcile <resourceVersion> [timeout_seconds]"
            exit 1
        fi

        echo "=== Waiting for OAO to reconcile ==="
        echo "Pre-tamper ocm-access-token resourceVersion: $PRE_RV"
        echo "Polling every ${INTERVAL}s, timeout ${TIMEOUT}s"
        echo ""

        while [ "$ELAPSED" -lt "$TIMEOUT" ]; do
            CURRENT_RV=$(ocm backplane elevate "$REASON" -- get secret ocm-access-token -n openshift-ocm-agent-operator -o jsonpath='{.metadata.resourceVersion}' 2>/dev/null)
            if [ -n "$CURRENT_RV" ] && [ "$CURRENT_RV" != "$PRE_RV" ]; then
                echo "OAO RECONCILED at ${ELAPSED}s"
                echo "  resourceVersion: $PRE_RV → $CURRENT_RV"

                # Check token hash
                TOKEN_HASH=$(ocm backplane elevate "$REASON" -- get secret ocm-access-token -n openshift-ocm-agent-operator -o jsonpath='{.data.access_token}' 2>/dev/null | base64 -d 2>/dev/null | shasum -a 256 | cut -c1-12)
                echo "  token_hash: $TOKEN_HASH"

                # Check pod start times (OAO should have restarted them)
                echo ""
                echo "=== Post-reconcile pod status ==="
                ocm backplane elevate "$REASON" -- get pods -n openshift-ocm-agent-operator -l app=ocm-agent -o wide 2>/dev/null || echo "(failed)"
                echo ""
                echo "=== Post-reconcile pod start times ==="
                ocm backplane elevate "$REASON" -- get pods -n openshift-ocm-agent-operator -l app=ocm-agent -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.startTime}{"\n"}{end}' 2>/dev/null || echo "(failed)"

                exit 0
            fi

            printf "."
            sleep "$INTERVAL"
            ELAPSED=$((ELAPSED + INTERVAL))
        done

        echo ""
        echo "OAO DID NOT RECONCILE within ${TIMEOUT}s"
        echo "  Current resourceVersion: ${CURRENT_RV:-unknown}"
        echo "  Expected change from: $PRE_RV"
        exit 1
        ;;

    *)
        echo "Unknown action: $ACTION" >&2
        exit 1
        ;;
esac
