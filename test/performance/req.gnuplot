# gnuplot -e "name='Requests during GET'; filename='merge/get-req.csv'; outfile='get-req.png'" test/performance/req.gnuplot
# gnuplot -e "name='Requests during POST'; filename='merge/create-req.csv'; outfile='create-req.png'" test/performance/req.gnuplot
set datafile separator ','

set title name

set xdata time
set timefmt "%Y-%m-%d"

set key autotitle columnhead
set key at screen 1, graph 1
set rmargin 30

set ylabel "Requests"

set terminal png enhanced size 1280,960
set output outfile
set multiplot layout 2,1

plot for [i=2:5] filename using 1:i with lines

unset title
plot for [i=6:6] filename using 1:i with lines
