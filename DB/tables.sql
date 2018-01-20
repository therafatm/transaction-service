CREATE TABLE Users (
    uid serial PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    money DOUBLE PRECISION NOT NULL
);

CREATE TABLE Stocks (
    sid serial PRIMARY KEY,
    username VARCHAR(50) REFERENCES Users(username),
    symbol VARCHAR(10) NOT NULL UNIQUE,
    shares INTEGER NOT NULL
);

CREATE TABLE Reservations (
    rid serial PRIMARY KEY,
    username VARCHAR(50) REFERENCES Users(username),
    symbol VARCHAR(10),
    type VARCHAR(10),
    shares INTEGER NOT NULL,
    face_value DOUBLE PRECISION NOT NULL,
    time BIGINT NOT NULL
);