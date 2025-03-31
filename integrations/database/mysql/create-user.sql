CREATE USER 'kronicler'@'%' IDENTIFIED BY 'Databas3Passw0rd';
GRANT ALL PRIVILEGES ON kronicler.* TO 'kronicler'@'%';
FLUSH PRIVILEGES;
