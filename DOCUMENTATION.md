# COPS Encrypted Secret Operator - Documentazione Completa

## Indice

1. [Sviluppi Recenti (v0.4)](#sviluppi-recenti-v04)
2. [Panoramica](#panoramica)
3. [CRD - EncryptedSecret](#crd---encryptedsecret)
4. [CRD - GeneratedSecret](#crd---generatedsecret)
5. [Ciclo del Reconcile](#ciclo-del-reconcile)
6. [Architettura](#architettura)
7. [Casi d'Uso](#casi-duso)
8. [Configurazione](#configurazione)
9. [Best Practices](#best-practices)

---

## Sviluppi Recenti (v0.4)

La versione 0.4 introduce **miglioramenti significativi** all'architettura e alle funzionalità dell'operatore:

### 🎯 Nuove Features

#### 1. **Supporto Multi-Protocollo (HTTP + gRPC)**
- **Prima**: Solo HTTP supportato
- **Adesso**: Supporto a `http` e `grpc` come protocolli
- **Implicazione**: È possibile integrarsi con servizi gRPC moderni (es. vault-mock, servizi Go/Rust native)

```yaml
# HTTP (legacy)
cryptoEndpoint:
  protocol: "http"
  address: "https://crypto.example.com/encrypt"

# gRPC (nuovo)
cryptoEndpoint:
  protocol: "grpc"
  address: "crypto.example.com:443"
  insecure: true  # TLS skip verification (dev only)
```

#### 2. **Struttura Endpoint Potenziata**
La nuova classe `Endpoint` sostituisce le stringhe semplici con un oggetto strutturato:

```go
type Endpoint struct {
  Protocol string  // "http" o "grpc"
  Address  string  // URL o hostname:port
  Insecure *bool   // Skip TLS verification (opzionale)
}
```

**Vantaggi**:
- ✅ Supporto native a TLS/insecure connection
- ✅ Protocol routing automatico
- ✅ Configurazione più esplicita e type-safe

#### 3. **Nuovo CRD GeneratedSecret**
Fino alla v0.3, era supportato solo l'encryption di dati esistenti. Adesso è possibile **generare** dinamicamente segreti:

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: GeneratedSecret
metadata:
  name: jwt-keypair
spec:
  type: "keypair"
  endpoint:
    protocol: "grpc"
    address: "keygen-service:9000"
  parameters:
    algorithm: "RSA"
    keySize: "2048"
  trigger:
    onCreate: true      # Genera alla creazione
    onRotate: true      # Rigenera alla rotazione
    onSpecChange: false # Non rigenerare se parametri cambiano
  rotationInterval: "720h"
  maxVersions: 5
```

**Trigger di Generazione**:
- `onCreate`: Genera il secret al momento della creazione del CRD
- `onRotate`: Rigenera il secret ogni `rotationInterval`
- `onSpecChange`: Rigenera se cambiano i `parameters`
- `schedule`: (Opzionale) Cron schedule per generazione custom

#### 4. **Versioning e Cleanup Robusto**
Migliorie al sistema di versioning:

```yaml
maxVersions: 5  # Mantiene sempre solo le ultime 5 versioni
```

**Cosa accade**:
```
v1 (oldest) ← Eliminato dopo che v6 viene creato
v2
v3
v4
v5
v6 (newest) ← Mantenuto sempre
```

#### 5. **RestartTargets per Coordinate Pod Restart**
Nuovo campo per riavviare automaticamente i deployment quando il secret cambia:

```yaml
spec:
  restartTargets:
    - deployment/app-main      # Riavvia questo deployment
    - deployment/worker        # E questo
```

**Meccanismo**: L'operatore aggiorna l'annotation `secret-rotated` nel pod spec per triggerare un rolling restart.

#### 6. **Endpoint Struct nel Reconcile Loop**
Il reconcile loop è stato **completamente riscritto** e ottimizzato:

**Vecchio flusso** (v0.3):
```go
Reconcile → createSecret → updateSecret → (senza versioning smart)
```

**Nuovo flusso** (v0.4):
```
Reconcile
├─ 1️⃣ TTL check → handleDeletion() se scaduto
├─ 2️⃣ Recupera Alias Secret
├─ 3️⃣ Calcola specHash
├─ 4️⃣ Decide se cifrare (needEncrypt)
├─ 5️⃣ Se serve, chiama Encrypt() tramite Endpoint
├─ 6️⃣ Calcola cipherHash
├─ 7️⃣ Calcola cipher-based hash (non spec-based!)
├─ 8️⃣ DECISION: rotate se:
│   ├─ !aliasExists (primo secret)
│   ├─ (needEncrypt && oldCipherHash ≠ newCipherHash) (dati realmente cambiano)
│   └─ rotationDue (tempo)
└─ 9️⃣ Requeue per prossima rotazione
```

**Innovazione Chiave**: Utilizza **cipher-hash** (non spec-hash) per decidere la rotazione. Questo significa:
- Se il servizio crypto è **deterministico** (es. AWS KMS) e i dati non cambiano → **NO rotazione** (evita churn!)
- Se il servizio è **non-deterministico** (es. random) → rotazione basata su output effettivo

#### 7. **gRPC Support Completo con Protobuf**
È stata aggiunta una completa implementazione gRPC:

```protobuf
service HelloGrpc {
    rpc Generate (GenerateRequest) returns (GenerateResponse) {}
    rpc Encrypt (EncryptRequest) returns (EncryptResponse) {}
    rpc Store (StoreRequest) returns (StoreResponse) {}
}
```

**File generati**:
- `hello.proto`: Definizioni protobuf
- `hello.pb.go`: Serializzazione protobuf (auto-generated)
- `hello_grpc.pb.go`: gRPC client/server (auto-generated)
- `client_test.go`: Test unit con mock gRPC server

#### 8. **Test Unit Robusti**
Aggiunti test completi per il client crypto:

```go
TestGenerateGRPCSuccess()      // Mock server, test happy path
TestGenerateGRPCConnectionError() // Test error handling
TestEncryptGRPCSuccess()         // Encryption via gRPC
TestEncryptWithGRPCProtocol()    // Dispatcher routing
```

### 📊 Statistiche dei Cambiamenti

| Aspetto | v0.3 | v0.4 | Cambiamento |
|---------|------|------|------------|
| **Protocolli Supportati** | 1 (HTTP) | 2 (HTTP + gRPC) | ✅ +100% |
| **CRD** | 1 (EncryptedSecret) | 2 (+ GeneratedSecret) | ✅ Generazione dinamica |
| **Endpoint Type** | `string` | `Endpoint struct` | ✅ Type-safe |
| **Hash Strategy** | spec-hash | cipher-hash | ✅ Smarter rotation |
| **Test Coverage** | Basic | Complete (gRPC included) | ✅ 15+ test cases |
| **Docker Version** | 0.1 | 0.4 | ✅ v0.4 |

### 🔄 Migration Guide (v0.3 → v0.4)

Se migrando da v0.3, aggiornare la YAML:

**Prima (v0.3)**:
```yaml
cryptoEndpoint: "https://vault.example.com/encrypt"
```

**Dopo (v0.4)**:
```yaml
cryptoEndpoint:
  protocol: "http"
  address: "https://vault.example.com/encrypt"
```

### ⚠️ Breaking Changes

1. **`cryptoEndpoint` è adesso un Endpoint struct** (non una stringa)
2. **EncryptedSecret controller**: Completamente riscritto (interno, non API)
3. **GeneratedSecret è nuovo**: Richiede nuovi RBAC rules

### ✨ Best Practices per v0.4

1. **Usa gRPC se disponibile**: Più efficiente per batch large di segreti
2. **Sfrutta GeneratedSecret**: Per genere dinamiche di password/token
3. **Imposta maxVersions**: Almeno 2-3 per quick rollback
4. **Monitora lastRotation**: Per alerting su rotazioni ritardate

---

## Panoramica

L'**Encrypted Secret Operator** è un operatore Kubernetes che automatizza la gestione e la rotazione di secret cifrati e generati dinamicamente. Consente di:

- **Cifrare i dati** tramite un servizio di crittografia esterno (API crypto)
- **Generare segreti** utilizzando servizi esterni (password, token, chiavi)
- **Rotare automaticamente** i secret in base a intervalli di tempo
- **Gestire versioni** di secret, mantenendo uno storico controllato
- **Riavviare automaticamente** i deployment quando il secret cambia
- **Impostare un TTL** (Time To Live) per l'auto-eliminazione dei secret
- **Validare l'integrità** dei dati tramite hash SHA256

### Componenti Chiave

| Componente | Ruolo |
|-----------|------|
| **EncryptedSecret (CRD)** | Definisce il secret da cifrare e gestire |
| **GeneratedSecret (CRD)** | Definisce il secret da generare dinamicamente |
| **Alias Secret** | Secret Kubernetes standard che punta all'ultima versione |
| **Version Secrets** | Secret Kubernetes con naming `<name>-v<numero>` |
| **Crypto/Generation Endpoint** | Servizio esterno che esegue crittografia o generazione |

---

## CRD - EncryptedSecret

### Struttura della Spec

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: my-secret
  namespace: default
spec:
  # Dati in chiaro da cifrare (key-value)
  data:
    username: admin
    password: mysecretpass

  # Endpoint del servizio di crittografia
  cryptoEndpoint:
    protocol: "http"
    address: "https://crypto-service.example.com/encrypt"
    insecure: false  # false = valida TLS, true = skip verification (dev only)

  # Intervallo di rotazione automatica (Go duration format)
  # Es: "24h", "30m", "1h30m"
  rotationInterval: "24h"

  # TTL del secret - dopo questo tempo il secret viene automaticamente eliminato
  # Es: "168h" (7 giorni), "720h" (30 giorni)
  ttl: "168h"

  # Numero massimo di versioni da mantenere
  # Le versioni più vecchie vengono eliminate
  maxVersions: 3

  # Target da riavviare quando il secret cambia
  # Supporta al momento solo: deployment/<name>
  restartTargets:
    - deployment/my-app
    - deployment/another-app
```

### Struttura dello Status

```yaml
status:
  # Timestamp dell'ultima rotazione
  lastRotation: "2026-03-21T10:30:00Z"

  # Secret Kubernetes che contiene il valore cifrato attuale
  activeSecret: "my-secret-v2"

  # Numero della versione attuale
  currentVersion: 2

  # Fase del ciclo di riconciliazione (campo riservato, non sempre popolato)
  phase: "Active"

  # secretName: campo riservato, non sempre utilizzato
```

---

## Ciclo del Reconcile

Il riconciliatore esegue un ciclo intelligente che coordina cifrazione, rotazione e gestione delle versioni. Ecco il flusso passo dopo passo:

### 📊 Diagramma di Flusso

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Recupera EncryptedSecret                                  │
│    Se non trovato → IgnoreNotFound                           │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 2. Controllo TTL (Time To Live)                              │
│    isDue(creationTime + ttl)?                                │
│    YES → handleDeletion() → Ritorna                          │
│    NO  → Continua                                            │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 3. Recupera Alias Secret                                     │
│    Alias = Secret Kubernetes con nome identico               │
│    aliasExists := (err == nil)                               │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 4. Calcola Hash Spec                                         │
│    specHash := SHA256(sorted spec.data keys+values)          │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 5. Verifica Necessità Cifratura                              │
│    needEncrypt = (!aliasExists ||                            │
│                   alias.Annotations["spec-hash"] ≠ specHash) │
│                                                               │
│    Se needEncrypt = true:                                    │
│      • Chiama crypto.Encrypt(cryptoEndpoint, data)           │
│      • Calcola newCipherHash                                 │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 6. Controllo Rotazione Temporale                             │
│    RotationInterval == "" ? rotationDue = false              │
│                            : rotationDue = (now >            │
│                               lastRotation + interval)       │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│ 7. Decisione Finale: RUOTA?                                  │
│                                                               │
│    rotate = !aliasExists ||                                  │
│              (needEncrypt &&                                 │
│               oldCipherHash ≠ newCipherHash) ||              │
│              rotationDue                                     │
└────────────────────┬────────────────────────────────────────┘
                     │
        ┌────────────┴─────────────┐
        │                          │
   NO   │                          │   YES
        │                          │
    ┌───▼──────┐          ┌────────▼──────────────────────────┐
    │ Fine     │          │ 8. Esegui Rotazione                │
    │ Requeue  │          │                                     │
    │ per TTL/ │          │ • createNewVersionWithData()        │
    │ Rotation │          │ • updateAliasSecretWithMetadata()   │
    │ Interval │          │ • restartTargets()                  │
    └──────────┘          │ • cleanupOldVersions()              │
                          │ • Incrementa CurrentVersion         │
                          │ • Update Status                     │
                          └────────────┬───────────────────────┘
                                       │
                          ┌────────────▼───────────────┐
                          │ 9. Requeue per Prossima    │
                          │    Rotazione (se presente   │
                          │    RotationInterval)        │
                          └────────────────────────────┘
```

### Dettagli dei Passaggi Critici

#### **Passaggio 2: Controllo TTL**

```go
isExpired(es) {
  if es.Spec.TTL == "" → false  (nessun TTL → non scade)
  ttl := ParseDuration(es.Spec.TTL)
  exp := es.CreationTimestamp.Add(ttl)
  return time.Now().After(exp)
}
```

Se il secret è scaduto:
- Elimina l'Alias Secret
- Elimina tutti i Version Secrets (con label `encrypted-secret: <name>`)
- Elimina il CRD EncryptedSecret
- Ritorna senza errori

#### **Passaggio 5: Calcolo Hash e Cifratura Intelligente**

Il sistema utilizza **due livelli di hash** per ottimizzare le operazioni:

1. **Spec Hash**: SHA256 del contenuto dei dati in spec
   - Esempio: sono cambiati i dati in input?
   
2. **Cipher Hash**: SHA256 del ciphertext
   - Esempio: è cambiato effettivamente il testo cifrato?

**Logica chiave**: Se il ciphertext non cambia (es. servizio crypto è deterministico), **NON viene effettuata una rotazione**. Questo evita churn inutile.

```go
// Calcolo spec hash
specHash := SHA256(sorted(data.keys + data.values))

// Calcolo cipher hash
if needEncrypt {
  newCipher := crypto.Encrypt(endpoint, data)
  newCipherHash := SHA256(sorted(newCipher.keys + newCipher.values))
}

// Decisione rotazione basata su ciphertext
rotate := oldCipherHash ≠ newCipherHash
```

#### **Passaggio 7: Trigger di Rotazione**

Ci sono **3 trigger** indipendenti:

| Trigger | Condizione | Esempio |
|---------|-----------|---------|
| **Primo Secret** | `!aliasExists` | Creazione del CRD |
| **Cambio Spec** | `needEncrypt && newCipherHash ≠ oldCipherHash` | Modifica `data:` in spec |
| **Rotazione Programmata** | `now > lastRotation + rotationInterval` | Ogni 24h |

#### **Passaggio 8: Esecuzione Rotazione**

Una volta deciso che serve rotare:

1. **Cripta** (se non già fatto): `newData = crypto.Encrypt(endpoint, spec.data)`

2. **Crea Version Secret**: Kubernetes Secret con naming `<name>-v<version>`
   - Labels: `encrypted-secret: <name>`
   - Annotations: `spec-hash`, `cipher-hash`, `secret-version`

3. **Aggiorna Alias**: Il Secret di nome `<name>` (senza suffisso) punta sempre all'ultima versione
   - Data: copia da Version Secret
   - Annotations: spec-hash, cipher-hash, checksum
   - **Effetto**: Le app leggono l'Alias, quindi automaticamente prendono i nuovi dati

4. **Riavvia Target**: Aggiorna annotation `secret-rotated` nei deployment per triggerare il restart

5. **Pulisce Versioni Vecchie**: Elimina i Version Secrets più antichi, mantenendo solo gli ultimi `maxVersions`

#### **Passaggio 9: Requeue per Rotazione Futura**

Se è specificato un `rotationInterval`:
```go
nextRotation := lastRotation + rotationInterval
requeue := nextRotation - now
return Result{RequeueAfter: requeue}, nil
```

Il controller si mette in standby e si auto-risveglia alla prossima rotazione.

---

## Architettura

### Flusso di Dati

```
┌─────────────────────┐
│  EncryptedSecret    │  (CRD: input utente)
│  (dati in chiaro)   │
└──────────┬──────────┘
           │
           │ Spec: data, cryptoEndpoint
           │
           ▼
    ┌──────────────────┐
    │ Crypto Endpoint  │  (servizio esterno)
    │ (es. AWS KMS,    │  Input: plaintext
    │  HashiCorp Vault)│  Output: ciphertext
    └────────┬─────────┘
             │
             │ ciphertext
             │
    ┌────────▼──────────────────┐
    │  Version Secret v<N>       │
    │  (Secret Kubernetes)       │
    │  Data: cifrato             │
    │  Labels: encrypted-secret  │
    │  Annotations: hash, version│
    └────────┬──────────────────┘
             │
             │ Copy data
             │
    ┌────────▼──────────────────┐
    │  Alias Secret              │
    │  (Secret Kubernetes)       │
    │  Name: <same as CRD>       │
    │  Data: ultima versione     │
    └────────┬──────────────────┘
             │
             │ OwnerReference
             │ (per cleanup)
             │
    ┌────────▼──────────────────┐
    │  Deployment (Apps)         │
    │  mountPath: secret/es      │
    │ (Volume da Alias Secret)   │
    │ Restart: annotation update │
    └────────────────────────────┘
```

### Risorse Kubernetes Coinvolte

Per ogni `EncryptedSecret` creato, l'operatore gestisce:

1. **Un Alias Secret** (`<name>`)
   - Punta ai dati cifrati correnti
   - Usato dai volume mount delle app

2. **Più Version Secrets** (`<name>-v1`, `<name>-v2`, ...)
   - Storico delle versioni
   - Max `maxVersions` per evitare accumulo

3. **Lo stesso EncryptedSecret**
   - Status aggiornato con metadati

---

## Casi d'Uso

### Caso d'Uso 1: Credenziali Applicazione con Rotazione Giornaliera

**Scenario**: Un'app PostgreSQL che deve cambiare password ogni 24 ore per compliance.

#### Creazione del Secret Cifrato

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: db-credentials
  namespace: production
spec:
  data:
    username: "postgres"
    password: "SecureP@ssw0rd123!"
    host: "postgres.example.com"
    port: "5432"

  cryptoEndpoint:
    protocol: "http"
    address: "https://vault.company.com/api/v1/encrypt"

  # Rotazione automatica ogni 24h
  rotationInterval: "24h"

  # Mantieni le ultime 2 versioni per rollback
  maxVersions: 2

  # Riavvia l'app quando il secret cambia
  restartTargets:
    - deployment/my-database-app
    - deployment/migration-runner
```

#### Utilizzo nel Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-database-app
  namespace: production
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-app:v1
        volumeMounts:
        - name: db-secret
          mountPath: /etc/secrets/db
          readOnly: true
        env:
        - name: DB_USER_FILE
          value: /etc/secrets/db/username
        - name: DB_PASS_FILE
          value: /etc/secrets/db/password

      volumes:
      - name: db-secret
        secret:
          # ✅ Reference all'Alias Secret (sempre punta all'ultima versione)
          secretName: db-credentials
          items:
          - key: username
            path: username
          - key: password
            path: password
```

#### Flusso di Rotazione

```
T=0:00   EncryptedSecret creato
         → Alias "db-credentials" creato (v1)
         → Deployment riavviato

T=24:00  Timer di rotazione scatta
         → Crypto.Encrypt() → nuovo hash ciphertext
         → Version Secret "db-credentials-v2" creato
         → Alias aggiornato (ora punta a v2)
         → Deployment riavviato
         → v1 mantenuto (maxVersions=2)

T=48:00  Nuova rotazione
         → Version Secret "db-credentials-v3" creato
         → Alias aggiornato (ora punta a v3)
         → Deployment riavviato
         → v1 eliminato (maxVersions=2 → solo v2 e v3)
```

**Benefici**:
- ✅ Password ruotata automaticamente
- ✅ App sempre legge l'ultima versione tramite Alias
- ✅ Storico disponibile per audit
- ✅ Rollback possibile in caso di problema

---

### Caso d'Uso 2: API Key con TTL (Auto-Scadenza)

**Scenario**: Credenziali temporanee per un batch job che deve scadere dopo 7 giorni se non rinnovate.

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: temp-api-key
  namespace: batch-jobs
spec:
  data:
    api_key: "sk_temp_abc123def456xyz"
    api_secret: "secret_789ghi012jkl"

  cryptoEndpoint:
    protocol: "http"
    address: "https://secrets-manager.company.com/encrypt"

  # Nessuna rotazione automatica (job temporaneo)
  rotationInterval: ""

  # Auto-eliminazione dopo 7 giorni
  ttl: "168h"

  # Nessun restart target (batch job)
  maxVersions: 1
```

#### Flusso Lifecycle

```
T=0              EncryptedSecret creato (creationTimestamp)

T=0 a 6d         Secret attivo e utilizzabile dal Job

T=7d             isExpired() ritorna true durante reconciliation
                 → Alias Secret     eliminato
                 → Version Secrets   eliminati
                 → EncryptedSecret   eliminato
                 ❌ Secret completamente rimosso

Tentativo Job    Fallisce perché secret è stato eliminato
→ Error handling nell'app
```

**Benefici**:
- ✅ Nessuna gestione manuale del cleanup
- ✅ TTL garantito anche se secret non viene rigenerato
- ✅ Compliance: credenziali temporanee hanno scadenza forzata

---

### Caso d'Uso 3: Multi-App Secret Share con Versioning

**Scenario**: Un secret cifrato condiviso da 3 microservizi in namespace diversi. Solo i dati cambiano, non la rotazione.

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: shared-config
  namespace: microservices
spec:
  data:
    api_endpoint: "https://api.internal.com"
    cache_host: "redis.internal.svc.cluster.local"
    cache_port: "6379"
    timeout_ms: "5000"
    retry_count: "3"

  cryptoEndpoint:
    protocol: "http"
    address: "https://vault.company.com/v1/encrypt"

  # Rotazione basata su scadenza temporale
  rotationInterval: "720h"

  # TTL ampio (90 giorni)
  ttl: "2160h"

  # Mantenere 5 versioni per debug
  maxVersions: 5

  # Restart 3 servizi
  restartTargets:
    - deployment/service-a
    - deployment/service-b
    - deployment/service-c
```

#### Flow di Update Dati

```
Versione 1 (Day 0)
├─ activeSecret: shared-config-v1
├─ Deployment A riparte
├─ Deployment B riparte
└─ Deployment C riparte

Scenario 1: Dati cambiano (es. API endpoint)
├─ Operator calcola newSpecHash ≠ oldSpecHash
├─ Chiama Encrypt → newCipherHash ≠ oldCipherHash
├─ Decide: RUOTA
├─ Crea Version 2
├─ Alias aggiornato
└─ I 3 Deployment si restartano automaticamente

Scenario 2: Tempo: 30 giorni dopo
├─ rotationDue = true (lastRotation + 720h < now)
├─ Chiama nuovamente Encrypt (stesso plaintext)
└─ Se ciphertext è deterministico:
    ├─ newCipherHash == oldCipherHash
    ├─ NO rotazione (evita churn)
    └─ ma Status.lastRotation aggiornato
```

**Benefici**:
- ✅ Coordinamento automatico tra 3 servizi
- ✅ Storico completo di tutte le versioni
- ✅ Evita restart non necessari grazie al cipher hash check
- ✅ TTL globale garantisce eventual cleanup

---

### Caso d'Uso 4: Integrazione con AWS Secrets Manager

**Scenario**: Secret Kubernetes che utilizza AWS KMS per la cifratura, con rotazione automatica.

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: aws-managed-secret
  namespace: aws-workloads
spec:
  data:
    # Dati in chiaro (non visibili in plain in etcd)
    aws_access_key_id: "AKIAIOSFODNN7EXAMPLE"
    aws_secret_access_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    region: "eu-west-1"

  # Endpoint custom che chiama AWS KMS per cifrare
  cryptoEndpoint:
    protocol: "http"
    address: "http://crypto-service.workloads.svc.cluster.local:8080/encrypt"
  # dietro le quinte usa AWS SDK con KMS ARN: arn:aws:kms:eu-west-1:123456789:key/deadbeef

  # Rotazione ogni 90 giorni per compliance AWS
  rotationInterval: "2160h"  # 90 days

  # TTL: 1 anno (compliance)
  ttl: "8760h"

  # Mantienere 12 versioni (una per mese)
  maxVersions: 12

  # Riavvia workload quando secret cambia
  restartTargets:
    - deployment/data-processor
    - deployment/backup-agent
```

#### Kubernetes Secret Generato

```yaml
# Alias Secret (visibile in cluster)
kind: Secret
metadata:
  name: aws-managed-secret
  namespace: aws-workloads
  annotations:
    spec-hash: "a1b2c3d4e5..."
    cipher-hash: "f6g7h8i9j0..."
    checksum: "sha256hash..."
data:
  # ✅ Ciphertext (cifrato via AWS KMS)
  aws_access_key_id: <base64 encrypted>
  aws_secret_access_key: <base64 encrypted>
  region: <base64 encrypted>

---

# Version Secret (storico per audit)
kind: Secret
metadata:
  name: aws-managed-secret-v5
  namespace: aws-workloads
  labels:
    encrypted-secret: aws-managed-secret
  annotations:
    secret-version: "5"
    spec-hash: "a1b2c3d4e5..."
    cipher-hash: "f6g7h8i9j0..."
    timestamp: "2026-03-21T10:30:00Z"
data:
  aws_access_key_id: <base64 encrypted>
  aws_secret_access_key: <base64 encrypted>
  region: <base64 encrypted>
```

---

## Configurazione

### Impostazioni di Rotazione

| Impostazione | Formato | Effetto | Esempio |
|-------------|---------|--------|---------|
| `rotationInterval` | Go `time.Duration` | Rotazione periodica | `"24h"`, `"30m"`, `"720h"` |
| `ttl` | Go `time.Duration` | Auto-eliminazione | `"168h"`, `"720h"`, `"1h"` |
| `maxVersions` | integer | Storico mantenuto | `1`, `3`, `10` |

> Nota: `maxVersions` deve essere impostato a `1` o più. Un valore pari a `0` può causare la rimozione di tutte le versioni esistenti.

#### Format Go Duration

```
ns  → nanosecond
us  → microsecond
ms  → millisecond
s   → second
m   → minute
h   → hour
d*  → day (non standard, usare 24h)

Esempi validi:
✅ "30s"         → 30 secondi
✅ "5m"          → 5 minuti
✅ "2h30m"       → 2 ore e 30 minuti
✅ "24h"         → 1 giorno
✅ "168h"        → 7 giorni (1 settimana)
✅ "720h"        → 30 giorni (1 mese)
✅ "8760h"       → 365 giorni (1 anno)

❌ "7d"          → ERRORE (format non supportato)
```

### Hash Calculation

Entrambi gli hash vengono calcolati sui **dati ordinati lexicograficamente**:

```go
// Pseudocode
func hash(data map[string]string) string:
  keys := sort(data.keys)
  h := SHA256()
  for each key in keys:
    h.write(key)
    h.write(data[key])
  return hex(h.sum())
```

**Implicazione**: Lo stesso set di dati produce sempre lo stesso hash, indipendentemente l'ordine originale.

### Crypto Endpoint

L'endpoint `/encrypt` riceve:

```json
POST /encrypt
{
  "data": {
    "username": "admin",
    "password": "secret"
  }
}
```

E ritorna:

```json
{
  "data": {
    "username": "encrypted_username_here",
    "password": "encrypted_password_here"
  }
}
```

---

## Best Practices

### 1. **Progettazione del Cycle Time**

```yaml
# ❌ Troppo frequente → churn eccessivo
rotationInterval: "1m"     # NO!

# ✅ Bilanciato
rotationInterval: "24h"    # Buono per la maggior parte dei casi

# ✅ Lunga scadenza
rotationInterval: "720h"    # Per credenziali stabili
```

### 2. **Gestione del TTL**

```yaml
# ❌ TTL uguale a rotationInterval → confusione
ttl: "24h"
rotationInterval: "24h"

# ✅ TTL >> rotationInterval (se rotazione implicita)
ttl: "2160h"
rotationInterval: "24h"
# Scadenza forzata dopo 90 giorni, ma ruota ogni 24h

# ✅ Solo TTL (nessuna rotazione, solo cleanup)
ttl: "168h"
rotationInterval: ""
# Dopo 7 giorni, tutto viene eliminato automaticamente
```

### 3. **maxVersions per Debugging**

```yaml
# ❌ maxVersions: 1 → nessun storico
maxVersions: 1

# ✅ Bilanciato
maxVersions: 3     # Almeno 2-3 versioni per rollback veloce

# ✅ Per audit trail completo
maxVersions: 12    # 1 versione per mese
```

### 4. **Restart Targets - Strategie**

```yaml
# Opzione 1: Restart singolo deployment
restartTargets:
  - deployment/my-app

# Opzione 2: Restart più servizi (chain di dipendenze)
restartTargets:
  - deployment/cache-warmer
  - deployment/main-app
  - deployment/background-worker

# Opzione 3: Nessun restart (solo update del secret)
restartTargets: []   # App che leggono il secret dinamicamente
```

### 5. **Namespacing e RBAC**

```yaml
# ✅ Ogni namespace ha il suo EncryptedSecret
namespace: production
---
namespace: staging
---
namespace: development

# Utilizzo RBAC per limitare accesso:
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: encryptedsecret-user
  namespace: production
rules:
- apiGroups: ["security.copsds.com"]
  resources: ["encryptedsecrets"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
```

### 6. **Monitoraggio e Observability**

```yaml
# ✅ Aggiungere labels per monitoraggio
metadata:
  name: important-secret
  labels:
    app: my-app
    criticality: high
    environment: production
    rotated-by: encrypted-secret-operator

# ✅ Controllare Status per diagnosticare
kubectl describe encryptedsecret important-secret -n production

# Output:
# Status:
#   Active Secret: important-secret-v5
#   Current Version: 5
#   Last Rotation: 2026-03-21T10:30:00Z
```

### 7. **Development vs Production**

```yaml
# Development: ciclo veloce, TTL breve
---
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: dev-secret
  namespace: development
spec:
  rotationInterval: "1h"
  ttl: "24h"
  maxVersions: 2

# Production: ciclo lungo, storico ampio
---
apiVersion: security.copsds.com/v1alpha1
kind: EncryptedSecret
metadata:
  name: prod-secret
  namespace: production
spec:
  rotationInterval: "720h"
  ttl: "8760h"
  maxVersions: 12
  restartTargets:
    - deployment/api-server
    - deployment/worker-1
    - deployment/worker-2
```

---

## Troubleshooting

### Secret non viene cifrato

```bash
# 1. Verificare che il CRD sia stato creato
kubectl get encryptedsecrets -n default

# 2. Controllare i log dell'operatore
kubectl logs -l app.kubernetes.io/name=encrypted-secret-operator -n operator-system

# 3. Verificare lo status del CRD
kubectl describe encryptedsecret my-secret -n default

# Cerca:
# - Phase
# - ActiveSecret
# - Events
```

### Rotazione non scatta

```bash
# Verificare che il RotationInterval sia valido
kubectl get encryptedsecret my-secret -n default -o yaml | grep rotationInterval

# Dovrebbe essere un Go duration valido (es. "24h")

# Se correctness è un problema, forzare riconciliazione:
kubectl annotate encryptedsecret my-secret \
  force-reconcile="$(date +%s)" \
  -n default --overwrite
```

### Deployment non si riavvia

```bash
# Verificare che restartTargets sia corretto
kubectl get encryptedsecret my-secret -n default -o yaml | grep -A5 restartTargets

# Formato: "deployment/<name>"

# Controllare che il deployment esista
kubectl get deployments -n default
```

---

## CRD - GeneratedSecret

Il `GeneratedSecret` permette di generare dinamicamente segreti utilizzando servizi esterni, come generatori di password, token o coppie di chiavi crittografiche.

### Struttura della Spec

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: GeneratedSecret
metadata:
  name: my-generated-secret
  namespace: default
spec:
  # Tipo di segreto da generare
  type: "keypair"

  # Endpoint del servizio di generazione
  endpoint: "https://keygen-service.example.com/generate"

  # Parametri specifici per il tipo
  parameters:
    algorithm: "RSA"
    keySize: "2048"
    format: "PEM"

  # Trigger per la generazione
  trigger:
    onCreate: true      # Genera alla creazione
    onRotate: true      # Genera alla rotazione
    onSpecChange: false # Non rigenerare se parametri cambiano
    schedule: ""        # Opzionale: cron schedule

  # Intervallo di rotazione
  rotationInterval: "720h"  # Ogni mese

  # Numero massimo di versioni
  maxVersions: 5
```

### Caso d'Uso: Generazione Coppia di Chiavi RSA

**Scenario**: Generare automaticamente una coppia di chiavi RSA per autenticazione JWT in un'applicazione.

```yaml
apiVersion: security.copsds.com/v1alpha1
kind: GeneratedSecret
metadata:
  name: jwt-keypair
  namespace: auth-service
spec:
  type: "keypair"
  endpoint: "https://crypto-service.internal/generate"
  parameters:
    algorithm: "RSA"
    keySize: "2048"
    format: "PEM"
    includePublic: "true"
  trigger:
    onCreate: true
    onRotate: true
    onSpecChange: false
  rotationInterval: "720h"  # Rotazione mensile
  maxVersions: 3
```

#### Utilizzo nel Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-service
spec:
  template:
    spec:
      containers:
      - name: auth
        volumeMounts:
        - name: jwt-keys
          mountPath: /etc/ssl/jwt
      volumes:
      - name: jwt-keys
        secret:
          secretName: jwt-keypair
```

#### Flusso di Generazione

```
T=0:00   GeneratedSecret creato
         → Chiama crypto-service.internal/generate
         → Riceve private_key e public_key
         → Crea Secret "jwt-keypair" con i dati
         → Alias punta alla versione corrente

T=720h  Rotazione scatta
         → Rigenera nuova coppia di chiavi
         → Crea versione v2
         → Aggiorna alias
         → Deployment può rileggere le nuove chiavi
```

**Benefici**:
- ✅ Chiavi generate automaticamente senza intervento manuale
- ✅ Rotazione periodica per sicurezza
- ✅ Versioning per audit e rollback
- ✅ Integrazione con servizi di crittografia esistenti

---

## Conclusione

L'Encrypted Secret Operator fornisce un'astrazione potente per gestire secret cifrati e generati dinamicamente in Kubernetes con:

- **Cifratura centralizzata**: Integrazione con servizi KMS esterni
- **Generazione dinamica**: Creazione automatica di password, token e chiavi
- **Rotazione automatica**: Basata su tempo o cambio dati
- **Versionamento**: Storico completo e rollback
- **Coordinamento**: Restart automatico dei deployment
- **Compliance**: TTL per auto-scadenza

È ideale per:
- Credenziali di database con rotazione regolare
- API keys temporanee con auto-cleanup
- Coppie di chiavi crittografiche per autenticazione
- Configurazioni condivise tra microservizi
- Generazione sicura di segreti senza intervento manuale
- Audit trail completo per compliance
