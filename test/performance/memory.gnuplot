# gnuplot -e "name='Memory during GET'; filename='merge/get-memory.csv'; outfile='get-memory.png'" test/performance/memory.gnuplot
# gnuplot -e "name='Memory during POST'; filename='merge/create-memory.csv'; outfile='create-memory.png'" test/performance/memory.gnuplot
set datafile separator ','

set title name

set xdata time
set timefmt "%Y-%m-%d"

set key autotitle columnhead
set key at screen 1, graph 1
set rmargin 30

set ylabel "Memory"
set ytics 0,1e6,1e10
set format y '%.0s%cB'

set terminal png enhanced size 1280,960
set output outfile
set multiplot layout 3,1

plot for [i=2:5] filename using 1:i with lines

unset title
plot for [i=6:9] filename using 1:i with lines
plot for [i=10:13] filename using 1:i with lines
