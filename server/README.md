Ovo je server čvora koji služi za komunikaciju s drugim čvorovima i za komunikaciju s lokalnim modelom.

Osnovni princip rada mu je zasnovan na **libp2p** knjižnici (https://docs.libp2p.io/concepts/introduction/overview/)
Go je izabran kao programski jezik jer ima dobru podršku ove knjižnice.

## Princip rada server-a

1. Pri samom početku čvor će generirat svoju multiadresu.
   Svaki čvor ima svoju [multiadresu](docs.libp2p.io/concepts/fundamentals/addressing/).
   Multiadresa enkapsulira IP adresu, port i identifikaciju čvora.
   Multiadresa osigurava sve informacije potrebne da bi čvorovi međusobno komunicirali i znali 100% da komuniciraju s pravim čvorom, a ne nekim imposterom.

2. Čvor se pretplaćuje na zajedničku temu u GossipSub-u
   Svaki čvor se pridružuje zajedničkom [GossipSub-u](https://docs.libp2p.io/concepts/pubsub/overview/)
   Preko tog sub-a, svaki čvor promovira svoje modele i pretplaćuje se za notifikacije modela s drugih čvorova.
   Iako na prvu se čini kao centraliziran sustav, nije, GossipSub je decentraliziran publish/subscribe sustav, objašnjenje:
   _libp2p currently uses a design called gossipsub. It is named after the fact that peers gossip to each other about which messages they have seen and use this information to maintain a message delivery network._

3. Čvor inicijalizira KDHT
   Ovo omogućava čvorovima da pronađu druge čvorove u mreži koristeći distrubirani hash table.

4. GRPC server se pokreće
   Koristi se GRPC za komunikaciju s lokalnim modelom, koji će svakih N epoha slati svoj lokalni model svom Go server-u preko GRPC-a.

5. Pokreće se zasebni thread za stream-anje modela

Ukratko, **primjer rada server-a nakon inicijalizacije**:

1. Server preko **GRPC-a** primi trenutačni model od svog lokalnog modela.
2. Server taj model publish-a svim drugim čvorovima preko **GossipSuba**.
3. Neki drugi čvor koji želi taj model, otvori stream našem serveru i naš server prenosi model preko stream-a koji smo otvorili u zasebnom thread-u.
4. Server čuva lokalne modele u **/shared/local-models**, a tuđe modele u **/shared/remote-models**.
