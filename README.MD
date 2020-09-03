## Manager of envoy proxy service

> run in one pod with envoy

listen kubernetes events and configure envoy proxy

> read annotations from k8s service objects; example (from api template):
```
    aria.io/proxy-config: |
      listener: acc
      h2: false
      routes:
      - match: { prefix: "/{{ .Tier }}/{{ .Name }}/"}
        route: { prefix_rewrite: "/", timeout: {{ .Timeout }} }
```

> another example (from ui template): 
```
    aria.io/proxy-config: |
      listener: acc
      h2: false
      routes:
      - match: { path: "/"}
        redirect: { path_redirect: "/ui/entry-point/" }        
      - match: { prefix: "/ui/"}
        route: { prefix_rewrite: "/", timeout: normal }
      - match: { prefix: "/media/"}
        route: { prefix_rewrite: "/media/", timeout: normal }
```
