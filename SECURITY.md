# Security Policy

## Supported versions

Sprout is an early-stage language.
Only the latest release receives security fixes.

## Reporting a vulnerability

Please do not open a public issue for a security problem.
Report it privately to the maintainers.
You can reach them through the GitHub security tab.

## What to include

- The version of Sprout you use.
- A minimal example that triggers the problem.
- The impact you believe the problem has.

## Response

A maintainer will reply within a reasonable time.
We treat reports confidentially until the fix is ready.

## Scope

Sprout is a language implementation, not a server.
The build tool writes files only where you ask it to.
The CLI reads the files you name on the command line.
The interpreter and the VM do not access the network.
