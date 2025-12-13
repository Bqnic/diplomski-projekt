supervisord.conf služi za paljenje potrebnih programa pri pokretanju docker kontenjera.

Što se mora upalit odmah pri početku:

1. GRPC serveri (na Go i Python strani)
2. Main-ovi (Go server i PyTorch model trerniranje)

Dodatni programi se mogu napisat za potrebno testiranje, npr. zaseban main kod grpc klijenta na python dijelu kako bi se mockala komunikacija slanja "modela".
