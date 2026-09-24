kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: __PLUGIN_HOST_PATH__
        containerPath: /opt/plugins
        selinuxRelabel: true
