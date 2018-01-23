CREATE TABLE Users (
    uid serial PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    money DOUBLE PRECISION NOT NULL
);

CREATE TABLE Stocks (
    sid serial PRIMARY KEY,
    username VARCHAR(50) REFERENCES Users(username),
    symbol VARCHAR(10) NOT NULL,
    shares INTEGER NOT NULL
);

CREATE TABLE Reservations (
    rid serial PRIMARY KEY,
    username VARCHAR(50) REFERENCES Users(username),
    symbol VARCHAR(10),
    type VARCHAR(10),
    shares INTEGER NOT NULL,
    amount DOUBLE PRECISION NOT NULL,
    time BIGINT NOT NULL
);

CREATE TABLE Triggers (
    tid serial PRIMARY KEY,
    username VARCHAR(50) REFERENCES Users(username),
    symbol VARCHAR(10) NOT NULL,
    type VARCHAR(10) NOT NULL,
    amount DOUBLE PRECISION,
    shares INTEGER,
    trigger_price DOUBLE PRECISION
);