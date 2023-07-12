**Sažetak**
Ovaj load balancer razvijen je u Go programskom jeziku, svi servisi su zajedno kontejnezirani (pomoću Dockera) pa je samo uključivanje svih servisa lakše. Sastoji se od 3 vrlo jednostavna servera. Njegova svrha je da sva 3 servera vrti pri ulasku korisnika na jednoj lokaciji. Korištena je metoda chooseServerWeightedRoundRobin za težinsko raspoređivanje, pri čemu serveri imaju definirane težine. Server 3 ima najveću težinu, server 2 ima težinu koja je dvostruko manja od servera 3, dok server 1 ima težinu koja je trostruko manja od servera 3. To rezultira time da je vjerojatnost preusmjeravanja na server 1 tri puta manja od vjerojatnosti preusmjeravanja na server 3. Postoji HealthCheck funkcija koja provjerava stanje svih servera svakih 5 sekundi, ako su serveri aktivni javlja da jesu, ako serveri nisu aktivni javlja da nisu. 
Server ima nekoliko ruta a neke od njih su:
* Dodavanje novih servera u load balancer putem metode AddServer i uklanjanje servera putem metode RemoveServer
* Prikaz svih aktivnih servera
* Uključivanje i isključivanje servera
* Traženje određenog servera po imenu 
* Praćenje različitih metrika korištenjem Prometheus biblioteke. Metrike se odnose na broj zahtjeva po ruti, količinu korištene memorije servera (svaki server ima podatke o memoriji koji su dohvaćeni na metrikama), broj trenutnih veza po serveru, ukupni broj zahtjeva po serveru i ukupni statusni kodovi odgovora
* Prikazivanje nasumičnog servera (potpuno nasumičnog, jednaka je mogućnost za dobivanje svakog servera)

Trebao sam load balancer spojiti posebno sa Prometheusom i posebno sa Zipkinom da bi mogli biti međusobno povezani. Prometheus ima posebno sučelje pomoću kojeg se mogu pratiti raznovrsne prije navedene metrike, tj. njihova statistika, podaci o pojedinom serveru ili podaci o svim serverima. Također i Zipkin ima posebno sučelje koje služi za praćenje distribuiranih sustava te koristi koncepte Tracinga kako bi se pratila putanja i vrijeme izvođenja zahtjeva kroz više servisa. Na primjer ja imam praćenje izvođenja vremena za svih 3 servisa na jednom mjestu što je moguće vizualno vidjeti.
