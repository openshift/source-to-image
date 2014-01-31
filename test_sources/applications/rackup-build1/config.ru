app = proc do |env|
    [ 200, {'Content-Type' => 'text/plain'}, ["Test STI rack application"] ]
end

run app
