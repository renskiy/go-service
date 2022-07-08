create table scores
(
    id         bigserial primary key,
    score      double precision not null,
    updated_at timestamptz      not null
);
