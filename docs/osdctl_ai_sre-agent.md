## osdctl ai sre-agent

Run SRE Agent for automated incident investigation

### Synopsis


  SRE Agent is an AI-powered tool that helps SREs triage alerts and diagnose issues.
  It automatically fetches incident details from PagerDuty, finds relevant SOPs,
  and executes diagnostic commands on clusters.


```
osdctl ai sre-agent [flags]
```

### Examples

```

  # Interactive mode (asks for confirmation at each step)
  osdctl ai sre-agent --pd-url "${PD_URL}"

  # Fully automated mode (no confirmations)
  osdctl ai sre-agent --pd-url "${PD_URL}" --auto-execute

  # Specify output directory for sre-agent files
  osdctl ai sre-agent --pd-url "${PD_URL}" --output /tmp/sre-agent-output

```

### Options

```
      --auto-execute    Fully automated mode without confirmations
  -h, --help            help for sre-agent
      --output string   Output directory for sre-agent files (default: current directory)
      --pd-url string   PagerDuty incident URL (required)
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl ai](osdctl_ai.md)	 - AI-powered tools for SRE automation

