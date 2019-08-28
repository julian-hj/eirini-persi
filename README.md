# eirini-persi


Hacked up persistence extension for Eirini

Note that the accompanying yaml creates a load balancer, but then in order to
make it work you have to:
- hard code the IP of the lb into the Dockerfile and re-push it
- Delete the MutatingWebhookConfig that got created
- Delete the secret that gets created in cf-workloads
- Delete the webhook pod
- Re-apply the config.
