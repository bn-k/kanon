-- A small lending library, the fixture of kan import: library/ is its expected output, written by hand.

-- An entity with a self reference, a BOOLEAN, a DATETIME always at midnight and a BLOB image.
CREATE TABLE Members (
  MemberID  INTEGER PRIMARY KEY,
  Name      TEXT NOT NULL,
  Joined    DATETIME NOT NULL,
  Active    BOOLEAN NOT NULL,
  Photo     BLOB,
  SponsorID INTEGER REFERENCES Members (MemberID)
);
INSERT INTO Members VALUES
  (1, 'Ada Lovelace', '2023-06-01 00:00:00.000', 1,
   CAST('<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>' || char(10) AS BLOB), NULL),
  (2, ' Charles Babbage ', '2024-01-15 00:00:00', 0, NULL, 1),
  (3, 'Grace Hopper', '2025-03-02', 1, NULL, 1);

-- An entity with a text key, a multi-line text, a boolean checked IN (0, 1) and BLOBs that are not all images.
CREATE TABLE Books (
  BookID    TEXT PRIMARY KEY,
  Title     TEXT NOT NULL,
  Summary   TEXT,
  Price     NUMERIC,
  Available INTEGER NOT NULL CHECK (Available IN (0, 1)),
  Sample    BLOB
);
INSERT INTO Books VALUES
  ('DUNE', 'Dune {1965}',
   'A desert planet.' || char(10) || '- first edition: Chilton' || char(10) || char(10) || 'See [[spice]] and {water}.',
   1250.5, 1, CAST('%PDF-1.4' || char(10) AS BLOB)),
  ('Le Petit Prince', 'Le Petit Prince', 'Un aviateur rencontre un petit prince.', 12, 0,
   CAST('Once upon a time.' || char(10) AS BLOB)),
  ('sicp', 'Structure and Interpretation of Computer Programs', '', NULL, 1, NULL);

-- A weak table: its key is its book plus a number.
CREATE TABLE Copies (
  BookID TEXT NOT NULL REFERENCES Books (BookID),
  CopyNo INTEGER NOT NULL,
  Shelf  TEXT NOT NULL,
  Bought DATE,
  PRIMARY KEY (BookID, CopyNo)
);
INSERT INTO Copies VALUES ('DUNE', 1, 'A|3', '2020-02-01'), ('DUNE', 2, 'B1', NULL), ('sicp', 1, 'C2', '2019-11-30');

-- An entity with a nullable foreign key, two foreign keys to the same table and a DATETIME with a time.
CREATE TABLE Loans (
  LoanID    INTEGER PRIMARY KEY,
  BookID    TEXT NOT NULL REFERENCES Books (BookID),
  MemberID  INTEGER REFERENCES Members (MemberID),
  CheckedBy INTEGER NOT NULL REFERENCES Members (MemberID),
  LentAt    DATETIME NOT NULL,
  Returned  BIT
);
INSERT INTO Loans VALUES
  (1, 'DUNE', 2, 1, '2026-09-30 00:00:00', 1),
  (2, 'sicp', 3, 1, '2026-10-01 09:05:30', 0),
  (3, 'Le Petit Prince', NULL, 3, '2026-10-22 14:30', NULL);

-- A join table.
CREATE TABLE MemberFavorites (
  MemberID INTEGER NOT NULL REFERENCES Members (MemberID),
  BookID   TEXT NOT NULL REFERENCES Books (BookID),
  PRIMARY KEY (MemberID, BookID)
);
INSERT INTO MemberFavorites VALUES (1, 'DUNE'), (1, 'sicp'), (3, 'sicp');

-- A table without a key: its rows are numbered.
CREATE TABLE Visits (
  MemberID INTEGER NOT NULL REFERENCES Members (MemberID),
  Day      DATE NOT NULL
);
INSERT INTO Visits VALUES (3, '2026-10-20'), (1, '2026-10-21');

CREATE VIEW Overdue AS SELECT * FROM Loans WHERE Returned = 0;
