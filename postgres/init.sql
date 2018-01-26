CREATE USER develop with password 'dev';
CREATE DATABASE liefbase;
CREATE extension postgis;
GRANT ALL PRIVILEGES ON DATABASE liefbase TO develop;
alter user develop with superuser;
