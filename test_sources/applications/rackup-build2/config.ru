app = proc do |env|
    [ 200, {'Content-Type' => 'text/plain'}, ["Test wharfie rack application"] ]
end

run app
