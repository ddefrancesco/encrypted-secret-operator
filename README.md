# encrypted-secret-operator

Encrypted Secret Operator per Kubernetes gestisce secret cifrati con un endpoint di crittografia esterno, versioning automatico, rotazione programmata e riavvio controllato dei deployment.

## Funzionalità

- CRD `EncryptedSecret` per definire i dati in chiaro e l'endpoint di encrypt
- Alias Secret che punta sempre all'ultima versione valida
- Version Secrets per conservare uno storico delle versioni
- Rotazione basata su intervallo oppure su modifica dei dati
- TTL per auto-eliminazione dell'intero ciclo di secret
- Riavvio automatico dei deployment quando il secret cambia

## Prerequisiti

- Go v1.24+
- Docker
- kubectl
- Accesso a un cluster Kubernetes compatibile

## Installazione

1. Costruisci e pubblica l'immagine:

```sh
make docker-build docker-push IMG=<your-registry>/encrypted-secret-operator:tag
```

2. Installa le CRD nel cluster:

```sh
make install
```

3. Esegui il controller nel cluster:

```sh
make deploy IMG=<your-registry>/encrypted-secret-operator:tag
```

> Se incontri errori RBAC, verifica di avere i permessi corretti o usa un account con privilegi adeguati.

## Esempio rapido

Applica un esempio reale dal sample:

```sh
kubectl apply -k config/samples/
```

Verifica lo stato del CR:

```sh
kubectl get encryptedsecret -A
kubectl describe encryptedsecret <name> -n <namespace>
```

## Disinstallazione

Rimuovi le risorse utente:

```sh
kubectl delete -k config/samples/
```

Rimuovi le CRD:

```sh
make uninstall
```

Rimuovi il controller:

```sh
make undeploy
```

## Contributing

Se vuoi contribuire:

- apri issue per bug o suggerimenti
- crea PR con patch chiare e descrizioni complete
- assicurati che i test di base passino prima di inviare la PR

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0.
See the License for the specific language governing permissions and
limitations under the License.

