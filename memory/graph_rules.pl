% graph_rules.pl — inference rules over the knowledge graph.
%
% memory-store-mcp supplies the facts and asks the query; this file supplies the
% rules. The facts are:
%
%   edge(From, To, Relation).            % without a date
%   edge_on(From, To, Relation, Date).   % with the date
%
% Everything derived here is answered, never stored: a rule that fires adds no
% row to graph_edges. It answers a question the stored edges imply but do not
% state, which is the difference between a pile of triples and a graph.
%
% The file is versioned in git and embedded in the binary, so the rules behind
% an answer are the rules at that commit. The inline Prolog this replaces was
% assembled by string concatenation in the middle of a Go request handler,
% where it could not be reviewed, tested or versioned on its own.

% ---------------------------------------------------------------------------
% What the graph implies
% ---------------------------------------------------------------------------

% Marriage holds both ways. The graph stores one direction; the other is
% implied. inverse_of fires only when the reverse edge is genuinely absent, so
% it reports a gap rather than repeating a fact that is already stored.
inverse(жена, муж).
inverse(муж, жена).

inverse_of(A, B, R, S) :-
    edge(A, B, R),
    inverse(R, S),
    \+ edge(B, A, S).

% Two people who were at the same place on the same day were there together.
% Both visits are usually recorded; the company between them is not.
together(A, B, P, D) :-
    edge_on(A, P, был_в, D),
    edge_on(B, P, был_в, D),
    A \= B.

% If someone ordered a dish while at a place, the place serves it. The food
% registry states this for the places it lists; this derives it for every
% recorded visit, including the ones recorded one by one.
serves(P, Dish, Date) :-
    edge_on(A, P, был_в, Date),
    edge_on(A, Dish, заказал, Date),
    A \= P, A \= Dish, P \= Dish.

% ---------------------------------------------------------------------------
% What the graph cannot hold
% ---------------------------------------------------------------------------

% These relations admit one value per subject. Two different values mean the
% graph asserts something that cannot be true, and a human has to look: either
% an extraction was wrong or a fact changed and the old edge was never retired.
functional(жена).
functional(муж).
functional(живёт_в).

contradiction(A, R, B, C) :-
    functional(R),
    edge(A, B, R),
    edge(A, C, R),
    B \= C.
