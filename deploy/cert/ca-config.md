```shell
cat > ca-config.json << EOF
{
    "signing":{
        "default":{
            "expiry":"8760h"
        },
        "profiles":{
            "server":{
                "usages":["siging","key encipherment","server auth","client auth"],
                "expiry": "8760h"
            }
        }
    }
}
EOF
cat > ca-csr.json << EOF
{
    "CN":"kebernetes",
    "key": {
        "algo": "rsa",
        "size": 2048
    },
    "names": [
        {
            "C": "CN",
            "ST": "Beijing",
            "L": "Beijing",
            "O": "K8s",
            "OU": "System"
        }
    ]
}
EOF
# 生成证书
cfssl gencert -initca ca-csr.json | cfssljson -bare ca
```



```shell
cat > server-csr.json << EOF
{
    "CN":"adminssion",
    "key": {
        "algo": "rsa",
        "size": 2048
    },
     "names": [
        {
            "C": "CN",
            "ST": "Beijing",
            "L": "Beijing",
            "O": "K8s",
            "OU": "System"
        }
    ]
}
EOF
# 生成服务端
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json \
 -hostname=admission-registry.default.svc -profile=server server-csr.json | cfssljson -bare server
```


```
kubectl create secret tls admission-registry-tls \
   --key=server-key.pem \
   --cert=server.pem
```
