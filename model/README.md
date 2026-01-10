Pokretanje skripte za treniranje modela:

python diabetes_train_single.py --epochs 20 --log-weight-stats

Ovo je više-manje sve što je potrebno, namjestite broj epoha i eventualno ako želite pratiti težine, ostali argumenti su tehničke stvari i neki hiperparametri koje ćemo izbrusiti s vremenom.

Na početku nakon pokretanja povlači se dataset pa mu treba malo duže, to ćemo poslije spremiti da ne mora povlačiti s interneta.

Svaku epohu sprema se state dict svih težina i cijelog modela u folder runs/diabetes_mlp/checkpoints, to se može promijeniti kako god je potrebno za server, i taj file nodeovi međusobno šalju. To se isto također sprema i u json formatu za lakše praćenje rada modela.