CREATE USER 'kubearchive'@'%' IDENTIFIED BY 'Databas3Passw0rd';
GRANT ALL PRIVILEGES ON kubearchive.* TO 'kubearchive'@'%';
FLUSH PRIVILEGES;
