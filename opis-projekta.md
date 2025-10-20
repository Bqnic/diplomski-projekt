# Decentralizirano učenje modela umjetne inteligencije bez centralnog poslužitelja

## Opis projekta

Cilj projekta je istražiti **potpuno decentraliziran pristup kolaborativnom učenju modela umjetne inteligencije**, koristeći **P2P (peer-to-peer)** mrežu bez centralnog poslužitelja.  
Za razliku od _federativnog učenja_ gdje postoji centralni agregator, u ovom sustavu svi čvorovi (nodeovi) ravnopravno razmjenjuju parametre modela i zajednički konvergiraju prema globalnom modelu.

---

## Model

Za eksperiment ćemo koristiti **MLP (Multilayer Perceptron)** model za **prepoznavanje znamenaka (MNIST dataset)**.  
Ovaj model je jednostavan, ali dovoljno kompleksan da se vidi korist od P2P učenja.

### Ideja:

-   Svaki čvor trenira svoj lokalni model nad djelomično različitim podacima (npr. jedan ima znamenke 0–3, drugi 4–6, treći 7–9).
-   Nakon nekoliko epoha lokalnog učenja, čvorovi razmjenjuju parametre modela i rade konzensus.
-   Time se postiže decentralizirano učenje

### Tehnologije:

-   **PyTorch**
-   **MNIST dataset**

---

## P2P mreža

### 1. Komunikacija između modela

-   Koristit ćemo **gRPC** kao osnovu za komunikaciju između čvorova.  
    Svaki čvor će biti i **server** i **klijent** – moći će primati i slati modele drugim čvorovima.
-   Definirat će se `.proto` datoteka koja opisuje servis i strukture za slanje poruka između modela.

#### Tehnologije:

-   **gRPC**
-   **Protocol Buffers** za serijalizaciju podataka

---

### 2. Peer Discovery (Network Layer)

-   Ova komponenta omogućuje čvorovima da otkriju druge čvorove u mreži.
-   Za potrebe simulacije moguće je:
    -   Koristiti statičku listu IP adresa/portova (npr. JSON konfiguracija svih čvorova),
    -   Ili za automatsko otkrivanje neku prikladnu knjižnicu

#### Tehnologije:

-   Početno: ručno definirana lista IP adresa
-   Automatsko otkrivanje: **`libp2p`** za Python

---

### 3. Konsenzus i sinkronizacija (Consensus Layer)

-   Nakon što čvor primi modele od svojih susjeda, treba ih spojiti u novi model.
-   Istražiti konzensus algoritme, za početak je prosjek dovoljno solidan.

## Ciklus rada

1. Treniraj lokalni model na vlastitim podacima (npr. 1–2 epohe).
2. Pošalji svoj model susjedima putem gRPC-a.
3. Primi modele od susjeda.
4. Konzensus algoritam.
5. Nastavi lokalno učenje s novim modelom.

## Arhitektura sustava

```mermaid
flowchart TD

    subgraph PeerA["Čvor A"]
        A1[Treniraj lokalni model (MLP)] --> A2[Pošalji model susjedima (gRPC)]
        A2 --> A3[Primi modele susjeda]
        A3 --> A4[Prosjek parametara (konsenzus)]
        A4 --> A1
    end

    subgraph PeerB["Čvor B"]
        B1[Treniraj lokalni model (MLP)] --> B2[Pošalji model susjedima (gRPC)]
        B2 --> B3[Primi modele susjeda]
        B3 --> B4[Prosjek parametara (konsenzus)]
        B4 --> B1
    end

    subgraph PeerC["Čvor C"]
        C1[Treniraj lokalni model (MLP)] --> C2[Pošalji model susjedima (gRPC)]
        C2 --> C3[Primi modele susjeda]
        C3 --> C4[Prosjek parametara (konsenzus)]
        C4 --> C1
    end

    PeerA <--> PeerB
    PeerB <--> PeerC
    PeerC <--> PeerA

    classDef peer fill:#1e3a8a,stroke:#1e3a8a,stroke-width:1px,color:white;
    class PeerA,PeerB,PeerC peer;
```
